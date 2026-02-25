package cmd

import (
	"errors"
	"testing"
)

func TestCollectAllPages_SinglePage(t *testing.T) {
	calls := 0
	items, next, err := collectAllPages[string](false, func(token string) (string, []string, error) {
		calls++
		return "next-token", []string{"a", "b"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 items, got %d", len(items))
	}
	if next != "next-token" {
		t.Errorf("want next-token, got %q", next)
	}
	if calls != 1 {
		t.Errorf("want 1 call when allPages=false, got %d", calls)
	}
}

func TestCollectAllPages_AllPages(t *testing.T) {
	page := 0
	items, next, err := collectAllPages[int](true, func(token string) (string, []int, error) {
		page++
		switch page {
		case 1:
			if token != "" {
				t.Errorf("page 1: want empty token, got %q", token)
			}
			return "p2", []int{1, 2}, nil
		case 2:
			if token != "p2" {
				t.Errorf("page 2: want %q, got %q", "p2", token)
			}
			return "p3", []int{3}, nil
		case 3:
			return "", []int{4, 5}, nil
		default:
			t.Errorf("unexpected call %d", page)
			return "", nil, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Errorf("want 5 items, got %d: %v", len(items), items)
	}
	for i, v := range items {
		if v != i+1 {
			t.Errorf("items[%d] = %d, want %d", i, v, i+1)
		}
	}
	if next != "" {
		t.Errorf("want empty nextPageToken, got %q", next)
	}
}

func TestCollectAllPages_EmptyResult(t *testing.T) {
	items, next, err := collectAllPages[string](true, func(_ string) (string, []string, error) {
		return "", nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("want 0 items, got %d", len(items))
	}
	if next != "" {
		t.Errorf("want empty next, got %q", next)
	}
}

func TestCollectAllPages_Error(t *testing.T) {
	wantErr := errors.New("api error")
	_, _, err := collectAllPages[string](true, func(_ string) (string, []string, error) {
		return "", nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("want %v, got %v", wantErr, err)
	}
}

func TestCollectAllPages_ErrorMidway(t *testing.T) {
	call := 0
	wantErr := errors.New("second page error")
	_, _, err := collectAllPages[int](true, func(_ string) (string, []int, error) {
		call++
		if call == 2 {
			return "", nil, wantErr
		}
		return "next", []int{1}, nil
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("want %v, got %v", wantErr, err)
	}
}
