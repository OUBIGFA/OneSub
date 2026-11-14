package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	lobeAction "github.com/lobehub/lobe-mygo/pkg/action"
	lobeOverlay "github.com/lobehub/lobe-mygo/pkg/overlay"
)

// Command availability and effects are shared by toolbar and context menus;
// Lobe owns their actual rendering, focus, keyboard and popup behavior.
type menuCommand struct {
	Key, Label, Icon, Hotkey                      string
	Disabled, Checkable, Checked, Danger, Divider bool
	Run                                           func()
}

func menuProps(commands []menuCommand, onOpen []func()) lobeOverlay.DropdownMenuProps {
	items := make([]lobeOverlay.MenuItem, len(commands))
	for i, command := range commands {
		key := command.Key
		if key == "" {
			key = command.Label
		}
		items[i] = lobeOverlay.MenuItem{
			Key: key, Label: command.Label, Icon: icons[command.Icon], Hotkey: command.Hotkey,
			Disabled: command.Disabled, Checkable: command.Checkable, Checked: command.Checked, Danger: command.Danger, Divider: command.Divider,
		}
	}
	props := lobeOverlay.DropdownMenuProps{Items: items}
	if len(onOpen) > 0 {
		props.OnOpen = onOpen[0]
	}
	return props
}

func runMenuCommand(chosen string, commands []menuCommand) {
	if chosen == "" {
		return
	}
	for _, command := range commands {
		key := command.Key
		if key == "" {
			key = command.Label
		}
		if key == chosen && !command.Disabled && !command.Divider && command.Run != nil {
			command.Run()
			return
		}
	}
}

func lobeMenuButton(c *ui.Context, label, glyph string, compact bool, build func() []menuCommand, onOpen ...func()) ui.Element {
	// The state holder occupies no layout space and is scoped to this call's
	// native identity, so identical menus in separate windows cannot collide.
	open := ui.Local(ui.Box(c).Absolute().Size(0, 0), "menu-open", func() bool { return false })
	var trigger ui.Element
	var commands []menuCommand
	props := menuProps(nil, onOpen)
	props.ItemsFunc = func() []lobeOverlay.MenuItem { commands = build(); return menuProps(commands, nil).Items }
	chosen := lobeOverlay.DropdownMenu(c, open, props, func(t ui.Element) {
		if compact {
			trigger = lobeAction.StyleActionIcon(c, t, icons[glyph], lobeAction.ActionIconProps{Title: label, IconLabel: locales.A11yIconOf(label)}).Size(em(c, 32), em(c, 32))
		} else {
			trigger = lobeAction.StyleButton(c, t, "", lobeAction.ButtonProps{}).Label(label).Children(func() {
				if glyph != "" && glyph != "chevron-down" {
					icon(c, glyph).Label(locales.A11yIconOf(label))
				}
				ui.Text(c, label).SingleLine()
				if glyph == "chevron-down" {
					icon(c, glyph).Label(locales.A11yIconOf(label))
				}
			})
		}
	})
	runMenuCommand(chosen, commands)
	return trigger
}

func lobeContextMenu(c *ui.Context, target ui.Element, build func() []menuCommand, onOpen ...func()) {
	var commands []menuCommand
	props := menuProps(nil, onOpen)
	props.ItemsFunc = func() []lobeOverlay.MenuItem { commands = build(); return menuProps(commands, nil).Items }
	runMenuCommand(lobeOverlay.ContextMenu(c, target, props), commands)
}
