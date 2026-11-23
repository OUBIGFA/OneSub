package main

import (
	"github.com/egoist/mygo/ui"
	lobeTheme "github.com/lobehub/lobe-mygo/pkg/theme"
)

// Lobe supplies the complete neutral palette and radii. Blue is OneSub's
// explicit selection/primary-action variant, not a second component theme.
// Native tables and split panes consume the same theme as Lobe controls.
func studioTheme(base *ui.Theme, highContrast bool) (ui.Theme, studio) {
	tokens := lobeTheme.Light()
	if base.Dark {
		tokens = lobeTheme.Dark()
	}
	t := *tokens.Theme()
	t.Font, t.FontSize = base.Font, bodySize*base.FontSize/14
	blue := ui.LightTheme()
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = blue.Accent, blue.AccentHover, blue.AccentPressed, blue.AccentText
	t.Selection, t.Focus = blue.Selection, blue.Focus
	// Status colors are text, not bright badge fills. Use Lobe's readable
	// text/active variants on the light canvas; dark tokens already contrast.
	if !base.Dark {
		t.Danger, t.Warning = tokens.VolcanoActive, tokens.WarningText.Mix(tokens.Text, 0.4)
	}
	t.Inverse, t.InverseText = tokens.BgSpotlight, tokens.Text
	if !base.Dark {
		// Native tooltip and toast surfaces use the inverse theme role. Keep
		// their contrast independent from the light controls underneath.
		t.Inverse, t.InverseText = lobeTheme.Dark().BgSpotlight, tokens.TextLightSolid
	}
	if highContrast {
		t.Border, t.TextMuted, t.Focus = t.Text, t.Text, t.Accent
	}
	return t, surfaceColors(&t, highContrast)
}

// Content, elevation and rules keep Lobe's surface roles in both appearances.
// There is no separate WinOne/Fluent palette underneath the new components.
func studioColors(c *ui.Context) studio {
	return surfaceColors(c.Theme(), c.Preferences().HighContrast)
}

func surfaceColors(t *ui.Theme, highContrast bool) studio {
	tokens := lobeTheme.Light()
	if t.Dark {
		tokens = lobeTheme.Dark()
	}
	look := studio{
		chrome: ui.Hex("#F1F1F3"), panel: tokens.BgContainer,
		hairline: tokens.BorderSecondary,
		selected: t.Accent.Alpha(0.12), warningSoft: tokens.WarningBg,
	}
	// BorderSecondary equals BgElevated in Lobe's dark palette. Use its
	// normal border token so separators remain visible on modal surfaces.
	if t.Dark {
		look.chrome, look.panel = ui.Hex("#141519"), ui.Hex("#25262B")
		look.hairline = tokens.Border
	}
	if highContrast {
		look.chrome, look.panel, look.hairline = t.Background, t.Background, t.Border
	}
	return look
}
