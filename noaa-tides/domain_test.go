package noaatides

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// The client's HTTP behaviour is covered in noaa-tides_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "noaatides" {
		t.Errorf("Scheme = %q, want noaatides", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "noaa-tides" {
		t.Errorf("Identity.Binary = %q, want noaa-tides", info.Identity.Binary)
	}
}

func TestClassify_stationID(t *testing.T) {
	typ, id, err := Domain{}.Classify("8443970")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if typ != "station" {
		t.Errorf("type = %q, want station", typ)
	}
	if id != "8443970" {
		t.Errorf("id = %q, want 8443970", id)
	}
}

func TestClassify_stationURL(t *testing.T) {
	typ, id, err := Domain{}.Classify("https://tidesandcurrents.noaa.gov/stationhome.html?id=8443970")
	if err != nil {
		t.Fatalf("Classify url: %v", err)
	}
	if typ != "station" {
		t.Errorf("type = %q, want station", typ)
	}
	if id != "8443970" {
		t.Errorf("id = %q, want 8443970", id)
	}
}

func TestClassify_badInput(t *testing.T) {
	_, _, err := Domain{}.Classify("not-a-station")
	if err == nil {
		t.Error("expected error on bad input, got nil")
	}
}

func TestClassify_empty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("expected error on empty input, got nil")
	}
}

func TestLocate_station(t *testing.T) {
	got, err := Domain{}.Locate("station", "8443970")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	want := "https://tidesandcurrents.noaa.gov/stationhome.html?id=8443970"
	if got != want {
		t.Errorf("Locate = %q, want %q", got, want)
	}
}

func TestLocate_badType(t *testing.T) {
	_, err := Domain{}.Locate("page", "foo")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}
