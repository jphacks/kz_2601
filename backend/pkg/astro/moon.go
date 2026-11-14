package astro

import (
	"math"
	"time"
)

// Phase is one of eight moon phases, in order through a lunation.
type Phase int

const (
	PhaseNew Phase = iota
	PhaseWaxingCrescent
	PhaseFirstQuarter
	PhaseWaxingGibbous
	PhaseFull
	PhaseWaningGibbous
	PhaseLastQuarter
	PhaseWaningCrescent
)

var phaseNames = [...]struct{ en, ja string }{
	{"new", "新月"},
	{"waxing_crescent", "三日月"},
	{"first_quarter", "上弦"},
	{"waxing_gibbous", "十三夜"},
	{"full", "満月"},
	{"waning_gibbous", "居待月"},
	{"last_quarter", "下弦"},
	{"waning_crescent", "有明月"},
}

// Name is the stable key, e.g. "first_quarter".
func (p Phase) Name() string { return phaseNames[p&7].en }

// NameJa is the Japanese label, e.g. "上弦".
func (p Phase) NameJa() string { return phaseNames[p&7].ja }

// MoonDay is the moon for one local calendar day.
type MoonDay struct {
	// Age is days since the last new moon, at local noon (as NAOJ tabulates it).
	Age float64
	// Phase is a principal phase (new, first quarter, full, last quarter) when
	// that instant falls within the day, otherwise the intermediate phase.
	Phase Phase
}

// Moon returns the moon's age at local noon and the phase for the day in loc.
func Moon(year int, month time.Month, day int, loc *time.Location) MoonDay {
	if loc == nil {
		loc = time.UTC
	}
	start := time.Date(year, month, day, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	noon := time.Date(year, month, day, 12, 0, 0, 0, loc)

	// The principal phase at or before the end of the day.
	k := math.Floor(lunationK(end)*4) / 4
	t := PrincipalPhase(k)
	for !t.Before(end) {
		k -= 0.25
		t = PrincipalPhase(k)
	}
	for {
		if next := PrincipalPhase(k + 0.25); next.Before(end) {
			k, t = k+0.25, next
			continue
		}
		break
	}
	q := int(math.Round((k-math.Floor(k))*4)) % 4 // 0 new, 1 FQ, 2 full, 3 LQ
	phase := Phase(q * 2)
	if t.Before(start) {
		phase++
	}
	return MoonDay{Age: MoonAge(noon), Phase: phase}
}

// MoonAge is the days elapsed at t since the latest new moon.
func MoonAge(t time.Time) float64 {
	k := math.Floor(lunationK(t))
	nm := PrincipalPhase(k)
	for nm.After(t) {
		k--
		nm = PrincipalPhase(k)
	}
	for {
		next := PrincipalPhase(k + 1)
		if next.After(t) {
			break
		}
		k, nm = k+1, next
	}
	return t.Sub(nm).Hours() / 24
}

// lunationK is Meeus' k (lunations since the 2000-01-06 new moon) for t, approximately.
func lunationK(t time.Time) float64 {
	return (julianDay(t) - 2451550.09766) / 29.530588861
}

// PrincipalPhase returns the UTC instant of the phase k (Meeus ch. 49):
// an integer k is a new moon, k+0.25 first quarter, k+0.5 full, k+0.75 last quarter.
func PrincipalPhase(k float64) time.Time {
	T := k / 1236.85
	T2, T3, T4 := T*T, T*T*T, T*T*T*T
	jde := 2451550.09766 + 29.530588861*k + 0.00015437*T2 - 0.000000150*T3 + 0.00000000073*T4
	E := 1 - 0.002516*T - 0.0000074*T2
	M := rad(2.5534 + 29.10535670*k - 0.0000014*T2 - 0.00000011*T3)
	Mp := rad(201.5643 + 385.81693528*k + 0.0107582*T2 + 0.00001238*T3 - 0.000000058*T4)
	F := rad(160.7108 + 390.67050284*k - 0.0016118*T2 - 0.00000227*T3 + 0.000000011*T4)
	O := rad(124.7746 - 1.56375588*k + 0.0020672*T2 + 0.00000215*T3)
	sin := math.Sin

	frac := math.Mod(k, 1)
	if frac < 0 {
		frac++
	}
	var c float64
	switch q := int(math.Round(frac * 4)); q {
	case 0, 2, 4:
		nc := []float64{-0.40720, 0.17241, 0.01608, 0.01039, 0.00739, -0.00514, 0.00208}
		if q == 2 {
			nc = []float64{-0.40614, 0.17302, 0.01614, 0.01043, 0.00734, -0.00515, 0.00209}
		}
		c = nc[0]*sin(Mp) + nc[1]*E*sin(M) + nc[2]*sin(2*Mp) + nc[3]*sin(2*F) +
			nc[4]*E*sin(Mp-M) + nc[5]*E*sin(Mp+M) + nc[6]*E*E*sin(2*M) -
			0.00111*sin(Mp-2*F) - 0.00057*sin(Mp+2*F) + 0.00056*E*sin(2*Mp+M) -
			0.00042*sin(3*Mp) + 0.00042*E*sin(M+2*F) + 0.00038*E*sin(M-2*F) -
			0.00024*E*sin(2*Mp-M) - 0.00017*sin(O) - 0.00007*sin(Mp+2*M) +
			0.00004*sin(2*Mp-2*F) + 0.00004*sin(3*M) + 0.00003*sin(Mp+M-2*F) +
			0.00003*sin(2*Mp+2*F) - 0.00003*sin(Mp+M+2*F) + 0.00003*sin(Mp-M+2*F) -
			0.00002*sin(Mp-M-2*F) - 0.00002*sin(3*Mp+M) + 0.00002*sin(4*Mp)
	default:
		c = -0.62801*sin(Mp) + 0.17172*E*sin(M) - 0.01183*E*sin(Mp+M) +
			0.00862*sin(2*Mp) + 0.00804*sin(2*F) + 0.00454*E*sin(Mp-M) +
			0.00204*E*E*sin(2*M) - 0.00180*sin(Mp-2*F) - 0.00070*sin(Mp+2*F) -
			0.00040*sin(3*Mp) - 0.00034*E*sin(2*Mp-M) + 0.00032*E*sin(M+2*F) +
			0.00032*E*sin(M-2*F) - 0.00028*E*E*sin(Mp+2*M) + 0.00027*E*sin(2*Mp+M) -
			0.00017*sin(O) - 0.00005*sin(Mp-M-2*F) + 0.00004*sin(2*Mp+2*F) -
			0.00004*sin(Mp+M+2*F) + 0.00004*sin(Mp-2*M) + 0.00003*sin(Mp+M-2*F) +
			0.00003*sin(3*M) + 0.00002*sin(2*Mp-2*F) + 0.00002*sin(Mp-M+2*F) -
			0.00002*sin(3*Mp+M)
		w := 0.00306 - 0.00038*E*math.Cos(M) + 0.00026*math.Cos(Mp) -
			0.00002*math.Cos(Mp-M) + 0.00002*math.Cos(Mp+M) + 0.00002*math.Cos(2*F)
		if q == 1 {
			c += w
		} else {
			c -= w
		}
	}

	// Planetary arguments.
	a := [14][2]float64{
		{299.77 + 0.107408*k - 0.009173*T2, 0.000325},
		{251.88 + 0.016321*k, 0.000165},
		{251.83 + 26.651886*k, 0.000164},
		{349.42 + 36.412478*k, 0.000126},
		{84.66 + 18.206239*k, 0.000110},
		{141.74 + 53.303771*k, 0.000062},
		{207.14 + 2.453732*k, 0.000060},
		{154.84 + 7.306860*k, 0.000056},
		{34.52 + 27.261239*k, 0.000047},
		{207.19 + 0.121824*k, 0.000042},
		{291.34 + 1.844379*k, 0.000040},
		{161.72 + 24.198154*k, 0.000037},
		{239.56 + 25.513099*k, 0.000035},
		{331.55 + 3.592518*k, 0.000023},
	}
	for _, x := range a {
		c += x[1] * sin(rad(x[0]))
	}

	tt := fromJulianDay(jde + c)
	return tt.Add(-deltaT(tt))
}

// deltaT is TT−UT (Espenak & Meeus polynomial for 2005–2050; within ~10 s
// of observed values, far below the minute resolution used here).
func deltaT(t time.Time) time.Duration {
	y := float64(t.Year()) + (float64(t.YearDay())-0.5)/365.25 - 2000
	return time.Duration((62.92 + 0.32217*y + 0.005589*y*y) * float64(time.Second))
}
