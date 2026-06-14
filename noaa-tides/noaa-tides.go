// Package noaatides is the library behind the noaa-tides command line:
// the HTTP client, request shaping, and the typed data models for the
// NOAA Tides and Currents API (api.tidesandcurrents.noaa.gov).
//
// The Client is the spine every command shares. It sets a real User-Agent,
// paces requests so a busy session stays polite, and retries the transient
// failures (429 and 5xx) that any public API throws under load.
package noaatides

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Host is the primary API host.
const Host = "api.tidesandcurrents.noaa.gov"

// Config holds all tuneable parameters for a Client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns the production configuration for the NOAA Tides API.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.tidesandcurrents.noaa.gov",
		UserAgent: "noaa-tides-cli/0.1.0 (github.com/tamnd/noaa-tides-cli)",
		Rate:      500 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Observation is one time-stamped measurement (water level, air temperature, etc.).
type Observation struct {
	Station string  `kit:"id" json:"station"` // from metadata.id or query param
	Time    string  `json:"time"`
	Value   float64 `json:"value"`
	Quality string  `json:"quality"` // q field or "predicted"
}

// Prediction is a hi/lo tide prediction point.
type Prediction struct {
	Station string  `kit:"id" json:"station"`
	Time    string  `json:"time"`
	Value   float64 `json:"value"`
	Type    string  `json:"type"` // "H" or "L"
}

// Station is a NOAA tide prediction station.
type Station struct {
	ID   string  `kit:"id" json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// Client talks to api.tidesandcurrents.noaa.gov over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client configured from cfg.
func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// --- wire shapes for JSON decoding ---

type wireMetadata struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Lat  string `json:"lat"`
	Lon  string `json:"lon"`
}

type wireDataPoint struct {
	T string `json:"t"`
	V string `json:"v"`
	F string `json:"f"`
	Q string `json:"q"`
}

type wireDataResponse struct {
	Metadata wireMetadata    `json:"metadata"`
	Data     []wireDataPoint `json:"data"`
	Error    *wireError      `json:"error"`
}

type wirePredPoint struct {
	T    string `json:"t"`
	V    string `json:"v"`
	Type string `json:"type"`
}

type wirePredResponse struct {
	Predictions []wirePredPoint `json:"predictions"`
	Error       *wireError      `json:"error"`
}

type wireStation struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"` // note: NOAA uses "lng" not "lon"
}

type wireStationsResponse struct {
	Count    int           `json:"count"`
	Stations []wireStation `json:"stations"`
}

type wireError struct {
	Message string `json:"message"`
}

// WaterLevel fetches observed water level data for a station over a date range.
func (c *Client) WaterLevel(ctx context.Context, station, beginDate, endDate, datum string) ([]*Observation, error) {
	u := fmt.Sprintf(
		"%s/api/prod/datagetter?product=water_level&station=%s&datum=%s&time_zone=GMT&units=english&application=noaa-tides-cli&format=json&begin_date=%s&end_date=%s",
		c.cfg.BaseURL, station, datum, beginDate, endDate,
	)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw wireDataResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("noaatides: decode water_level: %w", err)
	}
	if raw.Error != nil {
		return nil, fmt.Errorf("noaatides: API error: %s", raw.Error.Message)
	}
	stationID := raw.Metadata.ID
	if stationID == "" {
		stationID = station
	}
	out := make([]*Observation, 0, len(raw.Data))
	for _, d := range raw.Data {
		v, _ := strconv.ParseFloat(d.V, 64)
		out = append(out, &Observation{
			Station: stationID,
			Time:    d.T,
			Value:   v,
			Quality: d.Q,
		})
	}
	return out, nil
}

// Predictions fetches hi/lo tide predictions for a station over a date range.
func (c *Client) Predictions(ctx context.Context, station, beginDate, endDate string) ([]*Prediction, error) {
	u := fmt.Sprintf(
		"%s/api/prod/datagetter?product=predictions&station=%s&datum=MLLW&time_zone=GMT&units=english&interval=hilo&format=json&begin_date=%s&end_date=%s",
		c.cfg.BaseURL, station, beginDate, endDate,
	)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw wirePredResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("noaatides: decode predictions: %w", err)
	}
	if raw.Error != nil {
		return nil, fmt.Errorf("noaatides: API error: %s", raw.Error.Message)
	}
	out := make([]*Prediction, 0, len(raw.Predictions))
	for _, p := range raw.Predictions {
		v, _ := strconv.ParseFloat(p.V, 64)
		out = append(out, &Prediction{
			Station: station,
			Time:    p.T,
			Value:   v,
			Type:    p.Type,
		})
	}
	return out, nil
}

// AirTemperature fetches observed air temperature data for a station over a date range.
func (c *Client) AirTemperature(ctx context.Context, station, beginDate, endDate string) ([]*Observation, error) {
	u := fmt.Sprintf(
		"%s/api/prod/datagetter?product=air_temperature&station=%s&datum=MLLW&time_zone=GMT&units=english&format=json&begin_date=%s&end_date=%s",
		c.cfg.BaseURL, station, beginDate, endDate,
	)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw wireDataResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("noaatides: decode air_temperature: %w", err)
	}
	if raw.Error != nil {
		return nil, fmt.Errorf("noaatides: API error: %s", raw.Error.Message)
	}
	stationID := raw.Metadata.ID
	if stationID == "" {
		stationID = station
	}
	out := make([]*Observation, 0, len(raw.Data))
	for _, d := range raw.Data {
		v, _ := strconv.ParseFloat(d.V, 64)
		out = append(out, &Observation{
			Station: stationID,
			Time:    d.T,
			Value:   v,
			Quality: "measured",
		})
	}
	return out, nil
}

// Stations fetches the list of NOAA tide prediction stations.
func (c *Client) Stations(ctx context.Context, limit int) ([]*Station, error) {
	u := c.cfg.BaseURL + "/mdapi/prod/webapi/stations.json?type=tidepredictions&units=english"
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw wireStationsResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("noaatides: decode stations: %w", err)
	}
	out := make([]*Station, 0, len(raw.Stations))
	for _, s := range raw.Stations {
		out = append(out, &Station{
			ID:   s.ID,
			Name: s.Name,
			Lat:  s.Lat,
			Lon:  s.Lng,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// get fetches a URL with retry and pacing.
func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, u)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("noaatides: get %s: %w", u, lastErr)
}

func (c *Client) do(ctx context.Context, u string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
