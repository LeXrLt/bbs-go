package migrations

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAddKalshiSiteNavPreservesExistingEntries(t *testing.T) {
	original := `[{"title":"首页","url":"/topics","custom":{"visible":true}},{"title":"每日复盘","url":"/daily-review"},{"title":"其他","url":"/other"}]`
	updated, changed, err := addKalshiSiteNav(original)
	if err != nil || !changed {
		t.Fatalf("add nav: changed=%v err=%v", changed, err)
	}
	var entries []rawSiteNav
	if err := json.Unmarshal([]byte(updated), &entries); err != nil || len(entries) != 4 {
		t.Fatalf("entries: %v %v", entries, err)
	}
	if string(entries[0]["custom"]) != `{"visible":true}` || string(entries[2]["url"]) != `"/kalshi"` || string(entries[3]["url"]) != `"/other"` {
		t.Fatalf("existing entries changed: %s", updated)
	}
	again, changed, err := addKalshiSiteNav(updated)
	if err != nil || changed || again != updated {
		t.Fatalf("not idempotent: changed=%v err=%v", changed, err)
	}
}

func TestAddKalshiSiteNavRejectsInvalidConfiguration(t *testing.T) {
	for _, input := range []string{"null", "{}", "[null]", `[{"url":null}]`} {
		unchanged, changed, err := addKalshiSiteNav(input)
		if !errors.Is(err, errInvalidSiteNavs) || changed || unchanged != input {
			t.Fatalf("input=%s changed=%v err=%v", input, changed, err)
		}
	}
}
