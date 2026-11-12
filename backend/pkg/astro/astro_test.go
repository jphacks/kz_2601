package astro

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

// docs/research/astro-fixture.json is shared with iOS, Android and Mac.
const fixturePath = "../../../docs/research/astro-fixture.json"

type sunRow struct {
	ID        string  `json:"id"`
	Date      string  `json:"date"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	TZ        string  `json:"tz"`
	Sunrise   string  `json:"sunrise"`
	Sunset    string  `json:"sunset"`
	CivilDawn string  `json:"civilDawn"`
	CivilDusk string  `json:"civilDusk"`
}

type polarRow struct {
	sunRow
	PolarDay      bool `json:"polarDay"`
	PolarNight    bool `json:"polarNight"`
	CivilAllNight bool `json:"civilAllNight"`
	CivilNone     bool `json:"civilNone"`
}

type fixture struct {
	Meta struct {
		ToleranceMinutes     float64 `json:"toleranceMinutes"`
		MoonAgeToleranceDays float64 `json:"moonAgeToleranceDays"`
	} `json:"meta"`
	Sun     []sunRow   `json:"sun"`
	Polar   []polarRow `json:"polar"`
	MoonAge []struct {
		Date string  `json:"date"`
		TZ   string  `json:"tz"`
		Age  float64 `json:"age"`
	} `json:"moonAge"`
	MoonPhaseInstants []struct {
		Phase string `json:"phase"`
		JST   string `json:"jst"`
	} `json:"moonPhaseInstants"`
	MoonPhaseDays []struct {
		Date  string `json:"date"`
		TZ    string `json:"tz"`
		Phase string `json:"phase"`
	} `json:"moonPhaseDays"`
}

func loadFixture(t *testing.T) fixture {
	t.Helper()
	b, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fx fixture
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	if fx.Meta.ToleranceMinutes == 0 || fx.Meta.MoonAgeToleranceDays == 0 {
		t.Fatal("fixture meta: tolerances missing")
	}
	return fx
}

func parseDay(t *testing.T, date, tz string) (int, time.Month, int, *time.Location) {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	d, err := time.ParseInLocation(time.DateOnly, date, loc)
	if err != nil {
		t.Fatal(err)
	}
	return d.Year(), d.Month(), d.Day(), loc
}

// checkTime compares got with the fixture's HH:MM on the same local date.
func checkTime(t *testing.T, name, want string, got time.Time, tol float64) {
	t.Helper()
	if want == "" {
		if !got.IsZero() {
			t.Errorf("%s: want none, got %s", name, got.Format(time.RFC3339))
		}
		return
	}
	if got.IsZero() {
		t.Errorf("%s: want %s, got none", name, want)
		return
	}
	w, err := time.ParseInLocation("15:04", want, got.Location())
	if err != nil {
		t.Fatal(err)
	}
	w = time.Date(got.Year(), got.Month(), got.Day(), w.Hour(), w.Minute(), 0, 0, got.Location())
	if diff := got.Sub(w).Minutes(); math.Abs(diff) > tol {
		t.Errorf("%s: want %s, got %s (%+.2f min)", name, want, got.Format("15:04:05"), diff)
	}
}

func TestSunFixture(t *testing.T) {
	fx := loadFixture(t)
	if len(fx.Sun) == 0 {
		t.Fatal("no sun rows")
	}
	tol := fx.Meta.ToleranceMinutes
	for _, r := range fx.Sun {
		t.Run(r.ID, func(t *testing.T) {
			y, m, d, loc := parseDay(t, r.Date, r.TZ)
			s, err := Sun(y, m, d, r.Lat, r.Lng, loc)
			if err != nil {
				t.Fatal(err)
			}
			if s.PolarDay || s.PolarNight || s.CivilAllNight || s.CivilNone {
				t.Fatalf("unexpected polar flag: %+v", s)
			}
			checkTime(t, "sunrise", r.Sunrise, s.Sunrise, tol)
			checkTime(t, "sunset", r.Sunset, s.Sunset, tol)
			checkTime(t, "civilDawn", r.CivilDawn, s.CivilDawn, tol)
			checkTime(t, "civilDusk", r.CivilDusk, s.CivilDusk, tol)
			if !s.CivilDawn.Before(s.Sunrise) || !s.Sunrise.Before(s.SolarNoon) ||
				!s.SolarNoon.Before(s.Sunset) || !s.Sunset.Before(s.CivilDusk) {
				t.Errorf("events out of order: %+v", s)
			}
		})
	}
}

func TestSunPolarFixture(t *testing.T) {
	fx := loadFixture(t)
	if len(fx.Polar) == 0 {
		t.Fatal("no polar rows")
	}
	for _, r := range fx.Polar {
		t.Run(r.ID, func(t *testing.T) {
			y, m, d, loc := parseDay(t, r.Date, r.TZ)
			s, err := Sun(y, m, d, r.Lat, r.Lng, loc)
			if err != nil {
				t.Fatal(err)
			}
			if s.PolarDay != r.PolarDay || s.PolarNight != r.PolarNight ||
				s.CivilAllNight != r.CivilAllNight || s.CivilNone != r.CivilNone {
				t.Fatalf("flags: got day=%v night=%v civilAllNight=%v civilNone=%v, want %+v",
					s.PolarDay, s.PolarNight, s.CivilAllNight, s.CivilNone, r)
			}
			if (s.PolarDay || s.PolarNight) && (!s.Sunrise.IsZero() || !s.Sunset.IsZero()) {
				t.Errorf("polar day/night must not have sunrise/sunset: %+v", s)
			}
			checkTime(t, "civilDawn", r.CivilDawn, s.CivilDawn, fx.Meta.ToleranceMinutes)
			checkTime(t, "civilDusk", r.CivilDusk, s.CivilDusk, fx.Meta.ToleranceMinutes)
		})
	}
}

func TestSunInvalidLocation(t *testing.T) {
	for _, c := range [][2]float64{{91, 0}, {-91, 0}, {0, 181}, {math.NaN(), 0}} {
		if _, err := Sun(2026, time.June, 21, c[0], c[1], time.UTC); err != ErrInvalidLocation {
			t.Errorf("%v: err = %v", c, err)
		}
	}
}

func TestMoonAgeFixture(t *testing.T) {
	fx := loadFixture(t)
	for _, r := range fx.MoonAge {
		t.Run(r.Date, func(t *testing.T) {
			y, m, d, loc := parseDay(t, r.Date, r.TZ)
			got := Moon(y, m, d, loc).Age
			if math.Abs(got-r.Age) > fx.Meta.MoonAgeToleranceDays {
				t.Errorf("age: want %.1f, got %.3f", r.Age, got)
			}
		})
	}
}

func TestMoonPhaseInstantsFixture(t *testing.T) {
	fx := loadFixture(t)
	if len(fx.MoonPhaseInstants) == 0 {
		t.Fatal("no phase instants")
	}
	quarter := map[string]float64{"new": 0, "first_quarter": 0.25, "full": 0.5, "last_quarter": 0.75}
	for _, r := range fx.MoonPhaseInstants {
		t.Run(r.JST+"-"+r.Phase, func(t *testing.T) {
			want, err := time.Parse("2006-01-02T15:04Z07:00", r.JST)
			if err != nil {
				t.Fatal(err)
			}
			q, ok := quarter[r.Phase]
			if !ok {
				t.Fatalf("unknown phase %q", r.Phase)
			}
			k := math.Round(lunationK(want)-q) + q
			got := PrincipalPhase(k)
			if diff := got.Sub(want).Minutes(); math.Abs(diff) > fx.Meta.ToleranceMinutes {
				t.Errorf("want %s, got %s (%+.2f min)", want.UTC().Format(time.RFC3339), got.Format(time.RFC3339), diff)
			}
		})
	}
}

func TestMoonPhaseDaysFixture(t *testing.T) {
	fx := loadFixture(t)
	for _, r := range fx.MoonPhaseDays {
		t.Run(r.Date, func(t *testing.T) {
			y, m, d, loc := parseDay(t, r.Date, r.TZ)
			if got := Moon(y, m, d, loc).Phase; got.Name() != r.Phase {
				t.Errorf("phase: want %s, got %s", r.Phase, got.Name())
			}
		})
	}
}

func TestPhaseNames(t *testing.T) {
	for p := PhaseNew; p <= PhaseWaningCrescent; p++ {
		if p.Name() == "" || p.NameJa() == "" {
			t.Errorf("phase %d has no name", p)
		}
	}
	if PhaseFull.Name() != "full" || PhaseFull.NameJa() != "満月" {
		t.Errorf("full: %s %s", PhaseFull.Name(), PhaseFull.NameJa())
	}
}
