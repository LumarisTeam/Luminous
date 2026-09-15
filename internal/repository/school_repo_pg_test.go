//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"luminous/internal/config"
	"luminous/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupPGTest(t *testing.T) *PGSchoolRepository {
	t.Helper()

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Skipf("skipping PG integration test (config load failed): %v", err)
		return nil
	}
	dsn := cfg.Database.DSN
	if dsn == "" {
		t.Skipf("skipping PG integration test (no database DSN configured)")
		return nil
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping PG integration test (no database): %v", err)
		return nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping PG integration test (ping failed): %v", err)
		return nil
	}

	pool.Exec(ctx, "DROP TABLE IF EXISTS schools CASCADE")

	repo := &PGSchoolRepository{pool: pool}
	if err := repo.autoMigrate(ctx); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(ctx, "DROP TABLE IF EXISTS schools CASCADE")
		pool.Close()
	})

	return repo
}

func TestPGCreateAndFindAll(t *testing.T) {
	repo := setupPGTest(t)

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
	if len(all[0].Features) != 1 || all[0].Features[0] != model.FeatureTimetable {
		t.Fatalf("expected timetable feature, got %v", all[0].Features)
	}
}

func TestPGFindByCode(t *testing.T) {
	repo := setupPGTest(t)

	repo.Create(testCtx, &model.School{
		Code: "FIND", Name: "Find Me", Website: "https://find.edu",
		Features: []model.Feature{}, Enabled: true,
	})

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

func TestPGFindEnabled(t *testing.T) {
	repo := setupPGTest(t)

	repo.Create(testCtx, &model.School{Code: "ON", Name: "On", Website: "https://a.edu", Features: nil, Enabled: true})
	repo.Create(testCtx, &model.School{Code: "OFF", Name: "Off", Website: "https://b.edu", Features: nil, Enabled: false})

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

func TestPGUpdate(t *testing.T) {
	repo := setupPGTest(t)

	repo.Create(testCtx, &model.School{Code: "UPD", Name: "Old", Website: "https://old.edu", Features: nil, Enabled: true})

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

func TestPGDelete(t *testing.T) {
	repo := setupPGTest(t)

	repo.Create(testCtx, &model.School{Code: "DEL", Name: "Delete Me", Website: "https://del.edu", Features: nil, Enabled: true})

	if err := repo.Delete(testCtx, "DEL"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByCode(testCtx, "DEL")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestPGCreateDuplicate(t *testing.T) {
	repo := setupPGTest(t)

	school := &model.School{Code: "DUP", Name: "Dup", Website: "https://dup.edu", Features: nil, Enabled: true}
	if err := repo.Create(testCtx, school); err != nil {
		t.Fatal(err)
	}

	if err := repo.Create(testCtx, school); err == nil {
		t.Fatal("expected error for duplicate code")
	}
}

func TestPGCount(t *testing.T) {
	repo := setupPGTest(t)

	n, err := repo.Count(testCtx)
	if err != nil {
		t.Fatalf("Count empty: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	repo.Create(testCtx, &model.School{Code: "A", Name: "A", Website: "https://a.edu", Features: nil, Enabled: true})
	repo.Create(testCtx, &model.School{Code: "B", Name: "B", Website: "https://b.edu", Features: nil, Enabled: true})

	n, err = repo.Count(testCtx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}

func TestPGFindAllPagination(t *testing.T) {
	repo := setupPGTest(t)

	for _, code := range []string{"A", "B", "C", "D", "E"} {
		repo.Create(testCtx, &model.School{Code: code, Name: code, Website: "https://" + code + ".edu", Features: nil, Enabled: true})
	}

	// Page 1: first 2 items
	page1, err := repo.FindAll(testCtx, 0, 2)
	if err != nil {
		t.Fatalf("FindAll(0,2): %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2, got %d", len(page1))
	}

	// Page 2: next 2 items
	page2, err := repo.FindAll(testCtx, 2, 2)
	if err != nil {
		t.Fatalf("FindAll(2,2): %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2, got %d", len(page2))
	}
	if page2[0].Code == page1[0].Code || page2[0].Code == page1[1].Code {
		t.Fatal("page2 should not overlap with page1")
	}
}

func TestPGEduSystemURLRoundTrip(t *testing.T) {
	repo := setupPGTest(t)

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

	got, err := repo.FindByCode(testCtx, "EDU")
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if got.EduSystemURL != "https://jwc.edu.cn/jwglxt" {
		t.Fatalf("expected edu system url to persist, got %q", got.EduSystemURL)
	}

	got.EduSystemURL = "https://new-jwc.edu.cn"
	if err := repo.Update(testCtx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repo.FindByCode(testCtx, "EDU")
	if err != nil {
		t.Fatalf("FindByCode after update: %v", err)
	}
	if updated.EduSystemURL != "https://new-jwc.edu.cn" {
		t.Fatalf("expected updated edu system url, got %q", updated.EduSystemURL)
	}

	// FindEnabled shares the SELECT list and scanSchool, so it must agree.
	enabled, err := repo.FindEnabled(testCtx)
	if err != nil {
		t.Fatalf("FindEnabled: %v", err)
	}
	for _, s := range enabled {
		if s.Code == "EDU" && s.EduSystemURL != "https://new-jwc.edu.cn" {
			t.Fatalf("FindEnabled returned %q, want the updated value", s.EduSystemURL)
		}
	}
}
