package repository

import (
	"context"
	"errors"
	"os"
	"testing"

	"luminous/internal/model"
)

var testCtx = context.Background()

func tempFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "schools-*.json")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestCreateAndFindAll(t *testing.T) {
	repo, err := NewJSONSchoolRepository(tempFile(t))
	if err != nil {
		t.Fatal(err)
	}

	school := &model.School{
		Code:     "TEST",
		Name:     "Test University",
		Website:  "https://test.edu",
		Features: []model.Feature{model.FeatureTimetable},
		Enabled:  true,
	}

	if err := repo.Create(testCtx, school); err != nil {
		t.Fatalf("Create: %v", err)
	}

	all, err := repo.FindAll(testCtx, 0, 0)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 school, got %d", len(all))
	}
	if all[0].Code != "TEST" {
		t.Fatalf("expected TEST, got %s", all[0].Code)
	}
}

func TestFindByCode(t *testing.T) {
	repo, err := NewJSONSchoolRepository(tempFile(t))
	if err != nil {
		t.Fatal(err)
	}

	school := &model.School{
		Code:     "FIND",
		Name:     "Find Me",
		Website:  "https://find.edu",
		Features: []model.Feature{},
		Enabled:  true,
	}
	if err := repo.Create(testCtx, school); err != nil {
		t.Fatal(err)
	}

	found, err := repo.FindByCode(testCtx, "FIND")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if found.Name != "Find Me" {
		t.Fatalf("unexpected name: %s", found.Name)
	}

	_, err = repo.FindByCode(testCtx, "MISSING")
	if err == nil {
		t.Fatal("expected error for missing school")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFindEnabled(t *testing.T) {
	repo, err := NewJSONSchoolRepository(tempFile(t))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Create(testCtx, &model.School{Code: "ON", Name: "On", Website: "https://a.edu", Features: nil, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(testCtx, &model.School{Code: "OFF", Name: "Off", Website: "https://b.edu", Features: nil, Enabled: false}); err != nil {
		t.Fatal(err)
	}

	enabled, err := repo.FindEnabled(testCtx)
	if err != nil {
		t.Fatalf("FindEnabled: %v", err)
	}
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled, got %d", len(enabled))
	}
	if enabled[0].Code != "ON" {
		t.Fatalf("expected ON, got %s", enabled[0].Code)
	}
}

func TestUpdate(t *testing.T) {
	repo, err := NewJSONSchoolRepository(tempFile(t))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Create(testCtx, &model.School{Code: "UPD", Name: "Old", Website: "https://old.edu", Features: nil, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	updated := &model.School{Code: "UPD", Name: "New", Website: "https://new.edu", Features: nil, Enabled: false}
	if err := repo.Update(testCtx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	found, _ := repo.FindByCode(testCtx, "UPD")
	if found.Name != "New" {
		t.Fatalf("expected New, got %s", found.Name)
	}
	if found.Website != "https://new.edu" {
		t.Fatalf("expected new url, got %s", found.Website)
	}
}

func TestDelete(t *testing.T) {
	repo, err := NewJSONSchoolRepository(tempFile(t))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Create(testCtx, &model.School{Code: "DEL", Name: "Delete Me", Website: "https://del.edu", Features: nil, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(testCtx, "DEL"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = repo.FindByCode(testCtx, "DEL")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCreateDuplicate(t *testing.T) {
	repo, err := NewJSONSchoolRepository(tempFile(t))
	if err != nil {
		t.Fatal(err)
	}

	school := &model.School{Code: "DUP", Name: "Dup", Website: "https://dup.edu", Features: nil, Enabled: true}
	if err := repo.Create(testCtx, school); err != nil {
		t.Fatal(err)
	}

	err = repo.Create(testCtx, school)
	if err == nil {
		t.Fatal("expected error for duplicate code")
	}
}

func TestPersistenceAcrossInstances(t *testing.T) {
	path := tempFile(t)

	repo1, _ := NewJSONSchoolRepository(path)
	if err := repo1.Create(testCtx, &model.School{Code: "PERSIST", Name: "Persist", Website: "https://p.edu", Features: nil, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	repo2, err := NewJSONSchoolRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	found, err := repo2.FindByCode(testCtx, "PERSIST")
	if err != nil {
		t.Fatalf("school not persisted to disk: %v", err)
	}
	if found.Name != "Persist" {
		t.Fatalf("unexpected name: %s", found.Name)
	}
}

func TestCount(t *testing.T) {
	repo, err := NewJSONSchoolRepository(tempFile(t))
	if err != nil {
		t.Fatal(err)
	}

	n, err := repo.Count(testCtx)
	if err != nil {
		t.Fatalf("Count empty: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	if err := repo.Create(testCtx, &model.School{Code: "A", Name: "A", Website: "https://a.edu", Features: nil, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(testCtx, &model.School{Code: "B", Name: "B", Website: "https://b.edu", Features: nil, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	n, err = repo.Count(testCtx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}

func TestFindAllPagination(t *testing.T) {
	repo, err := NewJSONSchoolRepository(tempFile(t))
	if err != nil {
		t.Fatal(err)
	}

	for _, code := range []string{"A", "B", "C", "D", "E"} {
		if err := repo.Create(testCtx, &model.School{Code: code, Name: code, Website: "https://" + code + ".edu", Features: nil, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}

	all, err := repo.FindAll(testCtx, 0, 0)
	if err != nil {
		t.Fatalf("FindAll(0,0): %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5, got %d", len(all))
	}
}

func TestEduSystemURLRoundTrip(t *testing.T) {
	path := tempFile(t)
	repo, err := NewJSONSchoolRepository(path)
	if err != nil {
		t.Fatal(err)
	}

	school := &model.School{
		Code:         "EDU",
		Name:         "Edu U",
		Website:      "https://api.edu",
		EduSystemURL: "https://jwc.edu.cn/jwglxt",
		Features:     []model.Feature{},
		Enabled:      true,
	}
	if err := repo.Create(testCtx, school); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Reload from disk to prove the field survives persistence, not just the
	// in-memory map.
	reloaded, err := NewJSONSchoolRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.FindByCode(testCtx, "EDU")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if got.EduSystemURL != "https://jwc.edu.cn/jwglxt" {
		t.Fatalf("expected edu system url to persist, got %q", got.EduSystemURL)
	}

	got.EduSystemURL = ""
	if err := reloaded.Update(testCtx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	cleared, err := reloaded.FindByCode(testCtx, "EDU")
	if err != nil {
		t.Fatalf("FindByCode after update: %v", err)
	}
	if cleared.EduSystemURL != "" {
		t.Fatalf("expected edu system url to be cleared, got %q", cleared.EduSystemURL)
	}
}

func TestEduSystemURLDefaultsToEmptyWhenAbsent(t *testing.T) {
	path := tempFile(t)
	if err := os.WriteFile(path, []byte(`{"OLD":{"code":"OLD","name":"Old","website":"https://old.edu","features":[],"enabled":true}}`), 0644); err != nil {
		t.Fatal(err)
	}

	repo, err := NewJSONSchoolRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByCode(testCtx, "OLD")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if got.EduSystemURL != "" {
		t.Fatalf("expected empty edu system url for legacy record, got %q", got.EduSystemURL)
	}
}
