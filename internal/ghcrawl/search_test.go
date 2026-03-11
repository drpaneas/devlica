package ghcrawl

import (
	"testing"
	"time"
)

func TestMonthlyWindows(t *testing.T) {
	t.Run("single month", func(t *testing.T) {
		from := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
		to := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
		windows := monthlyWindows(from, to)
		if len(windows) != 1 {
			t.Fatalf("expected 1 window, got %d", len(windows))
		}
	})

	t.Run("spans two months", func(t *testing.T) {
		from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC)
		windows := monthlyWindows(from, to)
		if len(windows) != 2 {
			t.Fatalf("expected 2 windows, got %d", len(windows))
		}
		if !windows[0].from.Equal(from) {
			t.Errorf("first window from = %v, want %v", windows[0].from, from)
		}
	})

	t.Run("month end start stays calendar aligned", func(t *testing.T) {
		from := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
		to := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
		windows := monthlyWindows(from, to)
		if len(windows) != 3 {
			t.Fatalf("expected 3 windows, got %d", len(windows))
		}
		want := []timeWindow{
			{
				from: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
				to:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			},
			{
				from: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				to:   time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			},
			{
				from: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				to:   time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC),
			},
		}
		for i := range want {
			if !windows[i].from.Equal(want[i].from) || !windows[i].to.Equal(want[i].to) {
				t.Fatalf("window %d = {%s..%s}, want {%s..%s}",
					i,
					windows[i].from.Format("2006-01-02"),
					windows[i].to.Format("2006-01-02"),
					want[i].from.Format("2006-01-02"),
					want[i].to.Format("2006-01-02"))
			}
		}
	})

	t.Run("spans a year", func(t *testing.T) {
		from := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)
		windows := monthlyWindows(from, to)
		if len(windows) != 12 {
			t.Fatalf("expected 12 windows, got %d", len(windows))
		}
	})

	t.Run("from equals to", func(t *testing.T) {
		date := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		windows := monthlyWindows(date, date)
		if len(windows) != 1 {
			t.Fatalf("expected 1 window, got %d", len(windows))
		}
	})

	t.Run("from after to", func(t *testing.T) {
		from := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		to := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windows := monthlyWindows(from, to)
		if len(windows) != 1 {
			t.Fatalf("expected 1 window (degenerate case), got %d", len(windows))
		}
	})
}

func TestTimeWindowQualifier(t *testing.T) {
	w := timeWindow{
		from: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		to:   time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
	}
	if got := w.qualifier("created"); got != "created:2024-03-01..2024-03-31" {
		t.Errorf("qualifier(created) = %q, want %q", got, "created:2024-03-01..2024-03-31")
	}
	if got := w.qualifier("updated"); got != "updated:2024-03-01..2024-03-31" {
		t.Errorf("qualifier(updated) = %q, want %q", got, "updated:2024-03-01..2024-03-31")
	}
	if got := w.qualifier(""); got != "created:2024-03-01..2024-03-31" {
		t.Errorf("qualifier(empty) = %q, want %q", got, "created:2024-03-01..2024-03-31")
	}
}
