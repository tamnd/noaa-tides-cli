package noaatides_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	noaatides "github.com/tamnd/noaa-tides-cli/noaa-tides"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *noaatides.Client {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	cfg := noaatides.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return noaatides.NewClient(cfg)
}

const sampleWaterLevel = `{
	"metadata": {"id":"8443970","name":"Boston","lat":"42.3548","lon":"-71.0503"},
	"data":[
		{"t":"2024-01-01 00:00","v":"2.565","s":"0.049","f":"0,0,0,0","q":"v"},
		{"t":"2024-01-01 00:06","v":"2.612","s":"0.046","f":"0,0,0,0","q":"v"}
	]
}`

const samplePredictions = `{
	"predictions":[
		{"t":"2024-01-01 01:20","v":"0.595","type":"L"},
		{"t":"2024-01-01 07:48","v":"9.281","type":"H"},
		{"t":"2024-01-01 14:02","v":"0.312","type":"L"}
	]
}`

const sampleAirTemp = `{
	"metadata": {"id":"8443970","name":"Boston","lat":"42.3548","lon":"-71.0503"},
	"data":[
		{"t":"2024-01-01 00:00","v":"36.0","f":"0,0,0"},
		{"t":"2024-01-01 00:06","v":"35.8","f":"0,0,0"}
	]
}`

func sampleStations() string {
	type wireStation struct {
		ID   string  `json:"id"`
		Name string  `json:"name"`
		Lat  float64 `json:"lat"`
		Lng  float64 `json:"lng"`
	}
	type resp struct {
		Count    int           `json:"count"`
		Stations []wireStation `json:"stations"`
	}
	r := resp{Count: 2, Stations: []wireStation{
		{ID: "8443970", Name: "Boston", Lat: 42.3548, Lng: -71.0503},
		{ID: "1612340", Name: "Honolulu", Lat: 21.3069, Lng: -157.8674},
	}}
	b, _ := json.Marshal(r)
	return string(b)
}

func TestWaterLevel_userAgent(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua == "" {
			t.Error("request carried no User-Agent header")
		}
		if !strings.Contains(ua, "noaa-tides") {
			t.Errorf("User-Agent %q does not contain noaa-tides", ua)
		}
		_, _ = w.Write([]byte(sampleWaterLevel))
	})
	obs, err := c.WaterLevel(context.Background(), "8443970", "20240101", "20240101", "MLLW")
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) == 0 {
		t.Error("expected at least one observation")
	}
}

func TestWaterLevel_parseValues(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleWaterLevel))
	})
	obs, err := c.WaterLevel(context.Background(), "8443970", "20240101", "20240101", "MLLW")
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 {
		t.Fatalf("len(obs) = %d, want 2", len(obs))
	}
	if obs[0].Station != "8443970" {
		t.Errorf("Station = %q, want 8443970", obs[0].Station)
	}
	if obs[0].Time != "2024-01-01 00:00" {
		t.Errorf("Time = %q, want 2024-01-01 00:00", obs[0].Time)
	}
	if obs[0].Value != 2.565 {
		t.Errorf("Value = %v, want 2.565", obs[0].Value)
	}
	if obs[0].Quality != "v" {
		t.Errorf("Quality = %q, want v", obs[0].Quality)
	}
}

func TestPredictions_hiLo(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "interval=hilo") {
			t.Errorf("URL query %q missing interval=hilo", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(samplePredictions))
	})
	preds, err := c.Predictions(context.Background(), "8443970", "20240101", "20240103")
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) != 3 {
		t.Fatalf("len(preds) = %d, want 3", len(preds))
	}
	if preds[0].Type != "L" {
		t.Errorf("preds[0].Type = %q, want L", preds[0].Type)
	}
	if preds[1].Type != "H" {
		t.Errorf("preds[1].Type = %q, want H", preds[1].Type)
	}
	if preds[1].Value != 9.281 {
		t.Errorf("preds[1].Value = %v, want 9.281", preds[1].Value)
	}
}

func TestAirTemp_parseValues(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "product=air_temperature") {
			t.Errorf("URL query %q missing product=air_temperature", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(sampleAirTemp))
	})
	obs, err := c.AirTemperature(context.Background(), "8443970", "20240101", "20240101")
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 {
		t.Fatalf("len(obs) = %d, want 2", len(obs))
	}
	if obs[0].Value != 36.0 {
		t.Errorf("Value = %v, want 36.0", obs[0].Value)
	}
}

func TestStations_parseLngAsLon(t *testing.T) {
	stationsJSON := sampleStations()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "stations.json") {
			t.Errorf("URL path %q missing stations.json", r.URL.Path)
		}
		_, _ = w.Write([]byte(stationsJSON))
	})
	list, err := c.Stations(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].ID != "8443970" {
		t.Errorf("ID = %q, want 8443970", list[0].ID)
	}
	if list[0].Name != "Boston" {
		t.Errorf("Name = %q, want Boston", list[0].Name)
	}
	if list[0].Lon != -71.0503 {
		t.Errorf("Lon = %v, want -71.0503", list[0].Lon)
	}
}

func TestStations_limit(t *testing.T) {
	stationsJSON := sampleStations()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(stationsJSON))
	})
	list, err := c.Stations(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d with limit=1, want 1", len(list))
	}
}

func TestRetryOn503(t *testing.T) {
	var hits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(samplePredictions))
	}))
	defer ts.Close()

	cfg := noaatides.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := noaatides.NewClient(cfg)

	preds, err := c.Predictions(context.Background(), "8443970", "20240101", "20240103")
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) == 0 {
		t.Error("expected predictions after retry")
	}
	if hits != 3 {
		t.Errorf("server hits = %d, want 3", hits)
	}
}
