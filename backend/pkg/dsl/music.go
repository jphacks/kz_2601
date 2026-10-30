package dsl

// musicPlayer (#412): Apple Music playback on this device (MusicKit system
// player, so earphone / lock-screen controls work). allowRequests lets nearby
// devices running the same app send song requests over MultipeerConnectivity;
// audio itself is never relayed between devices. No entity: the queue lives in
// the system player.

func checkMusicPlayer(where string, c *Component, add func(string, ...any)) {
	if c.AutoAccept && !c.AllowRequests {
		add("%s: musicPlayer autoAccept needs allowRequests: true", where)
	}
	if c.Entity != "" || c.Field != "" {
		add("%s: musicPlayer plays Apple Music on the device; remove entity/field", where)
	}
}

// checkMusicPlayerCount: the device has one system player, so a second
// musicPlayer would only mirror the first.
func checkMusicPlayerCount(a *App, add func(string, ...any)) {
	n := 0
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]Component{c}, c.Children...) {
				if x.Type == CompMusicPlayer {
					n++
				}
			}
		}
	}
	if n > 1 {
		add("musicPlayer appears %d times; keep one (the device has a single player)", n)
	}
}
