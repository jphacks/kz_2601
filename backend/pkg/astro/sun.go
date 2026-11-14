// Package astro computes sunrise, sunset, civil twilight and the moon's age
// and phase without calling an external API (#1659).
//
// The sun follows the NOAA Solar Calculator equations
// (https://gml.noaa.gov/grad/solcalc/calcdetails.html); the moon's principal
// phases follow Meeus, Astronomical Algorithms (2nd ed.), chapter 49.
// Pure functions only: where the values get computed (feed or device) is #1643.
package astro

import (
	"errors"
	"math"
	"time"
)

// Altitudes of the sun's centre for each event, in degrees.
const (
	horizonSunrise = -0.833 // upper limb on the horizon with standard refraction
	horizonCivil   = -6.0
)

// SunTimes holds one local calendar day's sun events. A time is zero when
// the event does not happen that day; the flags say why.
type SunTimes struct {
	SolarNoon time.Time
	Sunrise   time.Time
	Sunset    time.Time
	CivilDawn time.Time
	CivilDusk time.Time

	// PolarDay: the sun stays above the horizon all day (no sunrise/sunset).
	PolarDay bool
	// PolarNight: the sun stays below the horizon all day (no sunrise/sunset).
	PolarNight bool
	// CivilAllNight: the sun never sinks below -6°, so there is no civil dawn/dusk.
	CivilAllNight bool
	// CivilNone: the sun never rises above -6°, so there is no civil dawn/dusk.
	CivilNone bool
}

// ErrInvalidLocation is returned for a latitude/longitude outside the globe.
var ErrInvalidLocation = errors.New("astro: latitude must be in [-90, 90] and longitude in [-180, 180]")

// Sun returns the sun events for the given calendar day in loc, at lat/lng
// (degrees, north and east positive). Times are in loc.
func Sun(year int, month time.Month, day int, lat, lng float64, loc *time.Location) (SunTimes, error) {
	if math.IsNaN(lat) || math.IsNaN(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return SunTimes{}, ErrInvalidLocation
	}
	if loc == nil {
		loc = time.UTC
	}
	noon := solarNoon(time.Date(year, month, day, 12, 0, 0, 0, loc), lng)
	out := SunTimes{SolarNoon: noon.In(loc)}

	var ok bool
	out.Sunrise, out.Sunset, out.PolarDay, out.PolarNight, ok = riseSet(noon, lat, lng, horizonSunrise)
	if ok {
		out.Sunrise, out.Sunset = out.Sunrise.In(loc), out.Sunset.In(loc)
	}
	out.CivilDawn, out.CivilDusk, out.CivilAllNight, out.CivilNone, ok = riseSet(noon, lat, lng, horizonCivil)
	if ok {
		out.CivilDawn, out.CivilDusk = out.CivilDawn.In(loc), out.CivilDusk.In(loc)
	}
	return out, nil
}

// solarNoon finds the transit nearest to t.
func solarNoon(t time.Time, lng float64) time.Time {
	for range 3 {
		t = t.Add(minutes(-hourAngle(t, lng) * 4))
	}
	return t
}

// riseSet returns the times the sun's centre crosses alt around noon.
// above/below report that it never crosses because it stays above/below.
func riseSet(noon time.Time, lat, lng, alt float64) (rise, set time.Time, above, below, ok bool) {
	_, dec := sunPosition(noon)
	cosH := eventCos(lat, dec, alt)
	switch {
	case cosH < -1:
		return time.Time{}, time.Time{}, true, false, false
	case cosH > 1:
		return time.Time{}, time.Time{}, false, true, false
	}
	h := deg(math.Acos(cosH))
	return refine(noon.Add(minutes(-h*4)), lat, lng, alt, -1),
		refine(noon.Add(minutes(h*4)), lat, lng, alt, 1), false, false, true
}

// refine moves t until the sun's hour angle matches the event's (sign -1 rise, +1 set).
func refine(t time.Time, lat, lng, alt, sign float64) time.Time {
	for range 4 {
		_, dec := sunPosition(t)
		cosH := math.Max(-1, math.Min(1, eventCos(lat, dec, alt)))
		want := sign * deg(math.Acos(cosH))
		t = t.Add(minutes((want - hourAngle(t, lng)) * 4))
	}
	return t
}

func eventCos(lat, dec, alt float64) float64 {
	return (math.Sin(rad(alt)) - math.Sin(rad(lat))*math.Sin(rad(dec))) /
		(math.Cos(rad(lat)) * math.Cos(rad(dec)))
}

// hourAngle is the sun's local hour angle at t in degrees, in [-180, 180).
func hourAngle(t time.Time, lng float64) float64 {
	eot, _ := sunPosition(t)
	u := t.UTC()
	utcMin := float64(u.Hour()*60+u.Minute()) + float64(u.Second())/60 + float64(u.Nanosecond())/6e10
	tst := math.Mod(utcMin+eot+4*lng, 1440)
	if tst < 0 {
		tst += 1440
	}
	return tst/4 - 180
}

// sunPosition returns the equation of time (minutes) and declination (degrees)
// at t, per the NOAA spreadsheet.
func sunPosition(t time.Time) (eot, dec float64) {
	jc := (julianDay(t) - 2451545) / 36525
	l0 := math.Mod(280.46646+jc*(36000.76983+jc*0.0003032), 360)
	m := 357.52911 + jc*(35999.05029-0.0001537*jc)
	e := 0.016708634 - jc*(0.000042037+0.0000001267*jc)
	c := math.Sin(rad(m))*(1.914602-jc*(0.004817+0.000014*jc)) +
		math.Sin(rad(2*m))*(0.019993-0.000101*jc) +
		math.Sin(rad(3*m))*0.000289
	omega := 125.04 - 1934.136*jc
	appLong := l0 + c - 0.00569 - 0.00478*math.Sin(rad(omega))
	meanObliq := 23 + (26+(21.448-jc*(46.815+jc*(0.00059-jc*0.001813)))/60)/60
	obliq := meanObliq + 0.00256*math.Cos(rad(omega))
	dec = deg(math.Asin(math.Sin(rad(obliq)) * math.Sin(rad(appLong))))

	y := math.Pow(math.Tan(rad(obliq/2)), 2)
	eot = 4 * deg(y*math.Sin(2*rad(l0))-
		2*e*math.Sin(rad(m))+
		4*e*y*math.Sin(rad(m))*math.Cos(2*rad(l0))-
		0.5*y*y*math.Sin(4*rad(l0))-
		1.25*e*e*math.Sin(2*rad(m)))
	return eot, dec
}

// julianDay of t (UT).
func julianDay(t time.Time) float64 {
	return float64(t.UnixNano())/86400e9 + 2440587.5
}

func fromJulianDay(jd float64) time.Time {
	return time.Unix(0, int64(math.Round((jd-2440587.5)*86400e9))).UTC()
}

func minutes(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }
func rad(d float64) float64           { return d * math.Pi / 180 }
func deg(r float64) float64           { return r * 180 / math.Pi }
