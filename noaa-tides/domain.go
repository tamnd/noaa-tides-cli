package noaatides

import (
	"context"
	"regexp"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes noaatides as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/noaa-tides-cli/noaa-tides"
//
// The init below registers it; the host then dereferences noaa-tides:// URIs by
// routing to the operations Register installs. The same Domain also builds the
// standalone noaa-tides binary (see cli.NewApp), so the binary and a host share
// one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the NOAA Tides driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "noaatides",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "noaa-tides",
			Short:  "Fetch tides, water levels, and weather from NOAA.",
			Long: `noaa-tides reads public NOAA Tides and Currents data over plain HTTPS,
shapes it into clean records, and prints output that pipes into the rest of
your tools. No API key required.

Commands:
  water-level   Observed water level readings for a station
  predictions   Hi/lo tide predictions for a station
  air-temp      Observed air temperature readings for a station
  stations      List all NOAA tide prediction stations`,
			Site: "tidesandcurrents.noaa.gov",
			Repo: "https://github.com/tamnd/noaa-tides-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:    "water-level",
		Group:   "read",
		List:    true,
		Summary: "Fetch observed water level readings for a station",
		URIType: "station",
		Args:    []kit.Arg{{Name: "station", Help: "station ID e.g. 8443970"}},
	}, waterLevel)

	kit.Handle(app, kit.OpMeta{
		Name:    "predictions",
		Group:   "read",
		List:    true,
		Summary: "Fetch hi/lo tide predictions for a station",
		URIType: "station",
		Args:    []kit.Arg{{Name: "station", Help: "station ID e.g. 8443970"}},
	}, predictions)

	kit.Handle(app, kit.OpMeta{
		Name:    "air-temp",
		Group:   "read",
		List:    true,
		Summary: "Fetch observed air temperature readings for a station",
		URIType: "station",
		Args:    []kit.Arg{{Name: "station", Help: "station ID e.g. 8443970"}},
	}, airTemp)

	kit.Handle(app, kit.OpMeta{
		Name:    "stations",
		Group:   "read",
		List:    true,
		Summary: "List all NOAA tide prediction stations",
		URIType: "station",
	}, stations)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- inputs ---

type waterLevelInput struct {
	Station   string  `kit:"arg" help:"station ID e.g. 8443970"`
	BeginDate string  `kit:"flag" help:"start date YYYYMMDD" default:"20240101"`
	EndDate   string  `kit:"flag" help:"end date YYYYMMDD" default:"20240101"`
	Datum     string  `kit:"flag" help:"datum: MLLW|NAVD|MSL|MHW" default:"MLLW"`
	Client    *Client `kit:"inject"`
}

type predictionsInput struct {
	Station   string  `kit:"arg" help:"station ID e.g. 8443970"`
	BeginDate string  `kit:"flag" help:"start date YYYYMMDD" default:"20240101"`
	EndDate   string  `kit:"flag" help:"end date YYYYMMDD" default:"20240103"`
	Client    *Client `kit:"inject"`
}

type airTempInput struct {
	Station   string  `kit:"arg" help:"station ID e.g. 8443970"`
	BeginDate string  `kit:"flag" help:"start date YYYYMMDD" default:"20240101"`
	EndDate   string  `kit:"flag" help:"end date YYYYMMDD" default:"20240101"`
	Client    *Client `kit:"inject"`
}

type stationsInput struct {
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func waterLevel(ctx context.Context, in waterLevelInput, emit func(*Observation) error) error {
	obs, err := in.Client.WaterLevel(ctx, in.Station, in.BeginDate, in.EndDate, in.Datum)
	if err != nil {
		return mapErr(err)
	}
	for _, o := range obs {
		if err := emit(o); err != nil {
			return err
		}
	}
	return nil
}

func predictions(ctx context.Context, in predictionsInput, emit func(*Prediction) error) error {
	preds, err := in.Client.Predictions(ctx, in.Station, in.BeginDate, in.EndDate)
	if err != nil {
		return mapErr(err)
	}
	for _, p := range preds {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func airTemp(ctx context.Context, in airTempInput, emit func(*Observation) error) error {
	obs, err := in.Client.AirTemperature(ctx, in.Station, in.BeginDate, in.EndDate)
	if err != nil {
		return mapErr(err)
	}
	for _, o := range obs {
		if err := emit(o); err != nil {
			return err
		}
	}
	return nil
}

func stations(ctx context.Context, in stationsInput, emit func(*Station) error) error {
	list, err := in.Client.Stations(ctx, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, s := range list {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

var stationIDRE = regexp.MustCompile(`^\d{7}$`)

// Classify turns a station ID or stationhome URL into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	// Accept a full tidesandcurrents URL like
	// https://tidesandcurrents.noaa.gov/stationhome.html?id=8443970
	if strings.Contains(input, "tidesandcurrents.noaa.gov") {
		if idx := strings.Index(input, "id="); idx >= 0 {
			rest := input[idx+3:]
			if end := strings.IndexAny(rest, "&# "); end >= 0 {
				rest = rest[:end]
			}
			if stationIDRE.MatchString(rest) {
				return "station", rest, nil
			}
		}
	}
	// Accept a bare numeric station ID.
	if stationIDRE.MatchString(input) {
		return "station", input, nil
	}
	return "", "", errs.Usage("unrecognized NOAA station reference: %q", input)
}

// Locate returns the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "station" {
		return "", errs.Usage("noaa-tides has no resource type %q", uriType)
	}
	return "https://tidesandcurrents.noaa.gov/stationhome.html?id=" + id, nil
}

func mapErr(err error) error {
	return err
}
