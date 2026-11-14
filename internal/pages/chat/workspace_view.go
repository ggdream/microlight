package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/internal/workspace"
	"github.com/ggdream/microlight/pkg/elastic"
)

type workspaceForm struct {
	engineName                                                       string
	engines                                                          []string
	chat                                                             *conversation
	target                                                           workspace.Target
	kind, containerMode, storageMode, image, source, secret, message string
	pendingHostKey                                                   string
	details, progress                                                string
	remember, busy                                                   bool
	containers                                                       []workspace.Container
	cancel                                                           context.CancelFunc
	containerAdvanced                                                containerAdvancedForm
}

func (app *chatApp) openWorkspaceForm(chat *conversation) {
	f := &workspaceForm{chat: chat, target: chat.target, kind: chat.target.Kind}
	f.secret = app.workspaceSecrets[f.target.Credential]
	if f.kind == "docker" {
		f.kind = "container"
	}
	app.workspaceForm = f
}

func (app *chatApp) openNewWorkspaceForm(kind string) {
	f := &workspaceForm{kind: kind, containerMode: "existing", storageMode: "bind", target: workspace.Target{Kind: kind}}
	app.workspaceForm = f
	if kind == "container" {
		app.refreshContainer(f)
	} else if kind == "remote" || kind == "microvm" {
		f.target.Port, f.target.Auth = "22", "key"
	}
}

func (app *chatApp) clearWorkspaceSelection() {
	app.forgetWorkspaceCredential(app.draftTarget.Credential)
	app.draftTarget = workspace.Target{}
	app.draftWorkspace, app.accessError = "", ""
}

func (app *chatApp) forgetWorkspaceCredential(id string) {
	if id == "" {
		return
	}
	delete(app.workspaceSecrets, id)
	_ = workspace.DeleteCredential(id)
}

func (app *chatApp) closeWorkspaceForm() {
	if f := app.workspaceForm; f != nil && f.cancel != nil {
		f.cancel()
	}
	app.workspaceForm = nil
}

func workspaceButton(c *ui.Context, label string, disabled, primary bool, click func()) {
	button := pointerButton(c, disabled).Label(label).Height(34).PaddingX(12).
		AlignItems(ui.Center).Justify(ui.Center).Radius(7)
	bg, fg := side, ink
	if disabled {
		fg = muted
	} else if primary {
		bg, fg = accent, white
	} else if button.Hovered() {
		bg = hover
	}
	button.Background(bg).Children(func() { ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(fg) }).OnClick(click)
}

func (app *chatApp) workspaceDialog(c *ui.Context) {
	f := app.workspaceForm
	if f == nil {
		return
	}
	if app.selected() != f.chat || app.settingsOpen {
		app.closeWorkspaceForm()
		return
	}
	title, glyph := f.target.Label(), "folder"
	switch f.kind {
	case "remote":
		title, glyph = locales.WorkspaceSSHTitle(), "desktop_signal"
	case "microvm":
		title, glyph = locales.WorkspaceMicroVMTitle(), "cube"
	case "e2b":
		title, glyph = locales.WorkspaceE2B(), "cube"
	case "desktop":
		glyph = "desktop_signal"
	case "docker", "container":
		title, glyph = locales.WorkspaceDockerTitle(), "cube"
	}
	_, windowHeight := c.Size()
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .38))
		panel.Label(title).Width(540).MaxWidthPercent(94).Padding(18).Gap(14).
			Background(white).Border(1, border).Radius(13)
		ui.Row(c.Key("header")).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Box(c).Size(34, 34).Radius(8).Background(tint).AlignItems(ui.Center).Justify(ui.Center).
				Children(func() { icon(c, glyph, accent, 18) })
			ui.Text(c, title).FontSize(18).FontWeight(680)
		})
		body := c.Key("body")
		scroll, offset := elastic.Scroll(body)
		scroll.FillWidth().MaxHeight(max(100, min(470, windowHeight-145))).Children(func() {
			offset.Apply(ui.Column(body)).FillWidth().Gap(12).Children(func() {
				tokenLabel := locales.WorkspaceCredential()
				switch f.kind {
				case "browser":
					tokenLabel = locales.WorkspaceBrowserToken()
				case "desktop":
					tokenLabel = locales.WorkspaceDesktopToken()
				case "e2b":
					tokenLabel = locales.WorkspaceE2BAPIKey()
				}
				if f.chat != nil {
					settings.Card(body.Key("summary"), locales.WorkspaceConnection(), func() {
						ui.Text(body, f.target.Label()).FontSize(13).FontWeight(620)
						if f.target.Directory != "" {
							ui.Text(body, f.target.Directory).FontSize(12).TextColor(muted).Selectable()
						}
					})
					if f.kind == "remote" || f.kind == "microvm" || f.kind == "browser" || f.kind == "desktop" || f.kind == "e2b" {
						settings.Card(body.Key("auth"), locales.WorkspaceAuthentication(), func() {
							settings.Field(body.Key("credential"), tokenLabel, "", &f.secret).Password().Disabled(f.busy)
							theme.Checkbox(body.Key("remember"), &f.remember, locales.WorkspaceRemember(), f.busy)
						})
					}
				} else if f.kind == "container" {
					app.containerWorkspaceFields(body, f)
				} else if f.kind == "e2b" {
					settings.Card(body.Key("e2b"), locales.WorkspaceConnection(), func() {
						settings.Field(body.Key("sandbox"), locales.WorkspaceE2BSandboxID(), "", &f.target.Resource).Disabled(f.busy)
						settings.Field(body.Key("directory"), locales.WorkspaceRemoteFolder(), "/home/user", &f.target.Directory).Disabled(f.busy)
						settings.Field(body.Key("key"), tokenLabel, "", &f.secret).Password().Disabled(f.busy)
						theme.Checkbox(body.Key("remember"), &f.remember, locales.WorkspaceRemember(), f.busy)
					})
				} else if f.kind == "browser" || f.kind == "desktop" {
					label, placeholder := locales.WorkspaceBrowserEndpoint(), "wss://…/devtools/browser/…"
					if f.kind == "desktop" {
						label, placeholder = locales.WorkspaceDesktopEndpoint(), "http://127.0.0.1:9420"
					}
					settings.Card(body.Key("endpoint"), locales.WorkspaceConnection(), func() {
						settings.Field(body.Key("endpoint"), label, placeholder, &f.target.Resource).Disabled(f.busy)
						settings.Field(body.Key("token"), tokenLabel, "", &f.secret).Password().Disabled(f.busy)
						theme.Checkbox(body.Key("remember"), &f.remember, locales.WorkspaceRemember(), f.busy)
					})
				} else {
					if f.kind == "microvm" {
						ui.Text(body, locales.WorkspaceMicroVMHint()).FontSize(12).TextColor(muted)
					}
					app.sshWorkspaceFields(body, f)
				}
				if f.pendingHostKey != "" {
					settings.Card(body.Key("fingerprint"), locales.WorkspaceFingerprint(), func() {
						ui.Text(body, f.pendingHostKey).FontSize(12).TextColor(ink).Selectable()
					})
				}
				if f.message != "" {
					workspaceError(body.Key("error"), f.message, f.details)
				}
			})
		})
		ui.Row(c.Key("footer")).FillWidth().Gap(8).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if f.busy && f.progress != "" {
				ui.Text(c, f.progress).Grow(1).MinWidth(0).FontSize(12).TextColor(muted)
			}
			workspaceButton(c.Key("cancel"), locales.Cancel(), false, false, app.closeWorkspaceForm)
			if f.pendingHostKey != "" {
				workspaceButton(c.Key("host-key"), locales.WorkspaceTrust(), f.busy, true, func() {
					f.target.HostKey, f.pendingHostKey = f.pendingHostKey, ""
					app.connectWorkspace(f)
				})
				return
			}
			label := locales.WorkspaceConnect()
			if f.chat != nil {
				label = locales.WorkspaceReconnect()
			} else if f.kind == "container" && f.containerMode == "image" {
				label = locales.WorkspaceCreate()
			}
			unavailable := f.kind == "container" && f.chat == nil && (f.target.Engine.Name == "" || (f.containerMode == "existing" && f.target.Container == ""))
			workspaceButton(c.Key("connect"), label, f.busy || unavailable, true, func() { app.connectWorkspace(f) })
		})
	})
	if !open {
		app.closeWorkspaceForm()
	}
}

func workspaceError(c *ui.Context, message, details string) {
	card := ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(8).Padding(10, 12).Radius(8).
		Background(white.Mix(errorFG, .045)).Border(1, white.Mix(errorFG, .2))
	expanded := ui.Local(card, struct{ Message, Details string }{message, details}, func() bool { return false })
	card.Children(func() {
		icon(c, "shield_error", errorFG, 16)
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, message).FontSize(12).FontWeight(600).TextColor(errorFG).FillWidth()
			if details == "" {
				return
			}
			label := locales.WorkspaceDetails()
			rotation := float32(0)
			if *expanded {
				rotation = 90
			}
			pointerButton(c.Key("details"), false).Label(label).Height(24).AlignSelf(ui.Start).
				AlignItems(ui.Center).Gap(4).Radius(4).Children(func() {
				ui.Icon(c, icons.SVG("ios_chevron_right")).Size(10, 10).Rotate(rotation).TextColor(muted)
				ui.Text(c, label).FontSize(11).TextColor(muted)
			}).OnClick(func() { *expanded = !*expanded })
			if *expanded {
				ui.Text(c.Key("detail-text"), details).FontSize(11).TextColor(muted).FillWidth().Selectable()
			}
		})
	})
}

func (f *workspaceForm) changed() {
	f.message, f.details, f.pendingHostKey, f.target.HostKey = "", "", "", ""
}

func workspaceSelect(c *ui.Context, label string, value *string, options []modelOption, disabled bool, changed func()) {
	selected := ""
	for _, option := range options {
		if option.id == *value {
			selected = option.label
			break
		}
	}
	workspaceSelectLabel(c, label, selected, value, options, disabled, changed)
}

func workspaceSelectLabel(c *ui.Context, label, selected string, value *string, options []modelOption, disabled bool, changed func()) {
	ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(610)
		control := ui.SelectBase(c.Key("select"), value)
		control.Trigger.Label(label).Value(selected).Disabled(disabled).Height(35).FillWidth().PaddingX(10).
			Gap(8).AlignItems(ui.Center).Justify(ui.SpaceBetween).Background(canvas).Border(1, border).Radius(7).
			Children(func() {
				ui.Text(c, selected).MinWidth(0).SingleLine().Ellipsis("…").FontSize(13)
				ui.Icon(c, icons.SVG("ios_chevron_right")).Size(10, 10).Shrink(0).Rotate(90).TextColor(muted)
			}).OnChange(changed)
		control.Popup(func(panel ui.Element) {
			panel.Padding(4).Gap(2).Background(white).Border(1, border).Radius(8)
			for _, option := range options {
				item := control.Item(option.id).Label(option.label).Padding(6, 8).Radius(5).Cursor(ui.CursorPointer)
				if item.Highlighted() || option.id == *value {
					item.Background(tint)
				}
				item.Children(func() { ui.Text(c, option.label).FontSize(13) })
			}
		})
	})
}

func (app *chatApp) containerWorkspaceFields(c *ui.Context, f *workspaceForm) {
	settings.Card(c.Key("connection"), locales.WorkspaceConnection(), func() {
		options := make([]modelOption, 0, len(f.engines))
		for _, name := range f.engines {
			label := name
			switch name {
			case "docker":
				label = "Docker"
			case "podman":
				label = "Podman"
			case "nerdctl":
				label = "containerd (nerdctl)"
			}
			options = append(options, modelOption{id: name, label: label})
		}
		if len(options) > 0 {
			workspaceSelect(c.Key("engine"), locales.WorkspaceContainerEngine(), &f.engineName, options, f.busy, func() { app.refreshContainer(f) })
		}
		workspaceSelect(c.Key("source"), locales.WorkspaceContainerSource(), &f.containerMode, []modelOption{
			{id: "existing", label: locales.WorkspaceExisting()}, {id: "image", label: locales.WorkspaceFromImage()},
		}, f.busy, f.changed)
		if f.containerMode == "image" {
			settings.Field(c.Key("image"), locales.WorkspaceImage(), "ubuntu:24.04", &f.image).Disabled(f.busy).OnChange(f.changed)
			if f.target.Engine.Name == "" {
				workspaceButton(c.Key("retry"), locales.WorkspaceRefresh(), f.busy, false, func() { app.refreshContainer(f) })
			}
			return
		}
		options = make([]modelOption, 0, len(f.containers))
		selected := locales.WorkspaceChooseContainer()
		if len(f.containers) == 0 {
			selected = locales.WorkspaceNoContainers()
		}
		for _, container := range f.containers {
			options = append(options, modelOption{id: container.ID, label: container.Name})
			if container.ID == f.target.Container {
				selected = container.Name
			}
		}
		ui.Row(c.Key("container-row")).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
			workspaceSelectLabel(c.Key("containers"), locales.WorkspaceRunningContainers(), selected, &f.target.Container, options, f.busy || len(options) == 0, f.changed)
			workspaceButton(c.Key("refresh"), locales.WorkspaceRefresh(), f.busy, false, func() { app.refreshContainer(f) })
		})
	})
	settings.Card(c.Key("storage"), locales.WorkspaceStorage(), func() {
		settings.Field(c.Key("dir"), locales.WorkspaceContainerFolder(), "/workspace", &f.target.Directory).Disabled(f.busy).OnChange(f.changed)
		if f.containerMode != "image" {
			return
		}
		workspaceSelect(c.Key("storage-mode"), locales.WorkspaceStorageMode(), &f.storageMode, []modelOption{
			{id: "bind", label: locales.WorkspaceBind()}, {id: "volume", label: locales.WorkspaceVolume()},
		}, f.busy, f.changed)
		if f.storageMode == "bind" {
			app.workspaceBrowseField(c.Key("mount"), f, locales.WorkspaceLocalFolder(), &f.source, true)
		}
	})
	if f.containerMode == "image" {
		app.containerAdvancedFields(c.Key("advanced"), f)
	}
}

func (app *chatApp) workspaceBrowseField(c *ui.Context, f *workspaceForm, label string, value *string, directory bool) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
		settings.Field(c.Key("path"), label, "", value).Disabled(f.busy).OnChange(f.changed)
		workspaceButton(c.Key("browse"), locales.WorkspaceBrowse(), f.busy, false, func() { app.pickRuntimePath(f, value, directory) })
	})
}

func (app *chatApp) sshWorkspaceFields(c *ui.Context, f *workspaceForm) {
	disabled := f.busy || f.pendingHostKey != ""
	settings.Card(c.Key("connection"), locales.WorkspaceConnection(), func() {
		ui.Row(c.Key("address")).FillWidth().Gap(8).Children(func() {
			settings.Field(c.Key("host"), locales.WorkspaceSSHHost(), "example.com", &f.target.Host).Disabled(disabled).OnChange(f.changed)
			ui.Column(c.Key("port-column")).Width(92).Shrink(0).Children(func() {
				settings.Field(c.Key("port"), locales.WorkspaceSSHPort(), "22", &f.target.Port).Disabled(disabled).OnChange(f.changed)
			})
		})
		settings.Field(c.Key("user"), locales.WorkspaceSSHUser(), "", &f.target.User).Disabled(disabled).OnChange(f.changed)
	})
	settings.Card(c.Key("authentication"), locales.WorkspaceAuthentication(), func() {
		workspaceSelect(c.Key("method"), locales.WorkspaceAuthMethod(), &f.target.Auth, []modelOption{
			{id: "key", label: locales.WorkspacePrivateKey()}, {id: "password", label: locales.WorkspacePassword()},
		}, disabled, func() { f.changed(); f.secret = "" })
		label := locales.WorkspacePassword()
		if f.target.Auth == "key" {
			app.workspaceBrowseField(c.Key("key"), f, locales.WorkspacePrivateKeyFile(), &f.target.KeyFile, false)
			label = locales.WorkspacePassphrase()
		}
		settings.Field(c.Key("credential"), label, "", &f.secret).Password().Disabled(disabled)
		theme.Checkbox(c.Key("remember"), &f.remember, locales.WorkspaceRemember(), disabled)
	})
	settings.Card(c.Key("folder"), locales.WorkspaceStorage(), func() {
		settings.Field(c.Key("dir"), locales.WorkspaceRemoteFolder(), locales.WorkspaceRemoteFolderHint(), &f.target.Directory).Disabled(disabled).OnChange(f.changed)
	})
}

func (app *chatApp) pickRuntimePath(f *workspaceForm, value *string, directory bool) {
	if f.busy {
		return
	}
	f.busy = true
	current := *value
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: app.win, Title: locales.Choose(), DefaultPath: current, Directory: directory})
		app.win.Update(func() {
			if app.workspaceForm != f {
				return
			}
			f.busy = false
			if err != nil {
				f.message = err.Error()
				return
			}
			if len(paths) == 0 {
				return
			}
			*value = paths[0]
			f.changed()
		})
	}()
}

func (app *chatApp) refreshContainer(f *workspaceForm) {
	f.busy = true
	f.message, f.details, f.progress = "", "", locales.WorkspaceLoadingContainers()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	f.cancel = cancel
	preferred := f.engineName
	go func() {
		defer cancel()
		engines := workspace.ContainerEngines()
		var engine workspace.ContainerEngine
		var containers []workspace.Container
		var err error
		if preferred != "" {
			engine, containers, err = workspace.ListContainers(ctx, preferred)
		} else {
			err = errors.New(locales.WorkspaceContainerEngineMissing())
			for _, name := range engines {
				engine, containers, err = workspace.ListContainers(ctx, name)
				if err == nil {
					break
				}
			}
		}
		app.win.Update(func() {
			if app.workspaceForm != f {
				return
			}
			f.busy = false
			f.engines = engines
			if f.engineName == "" && len(engines) > 0 {
				f.engineName = engines[0]
			}
			if err != nil {
				f.target.Engine, f.target.Container, f.containers = workspace.ContainerEngine{}, "", nil
				f.message, f.details = locales.WorkspaceDockerListFailed(), err.Error()
				return
			}
			if engine != f.target.Engine {
				f.target.Container = ""
			}
			f.engineName, f.target.Engine, f.containers, f.message = engine.Name, engine, containers, ""
		})
	}()
}

func (app *chatApp) connectWorkspace(f *workspaceForm) {
	if f.busy {
		return
	}
	target, secret := f.target, f.secret
	if f.chat == nil {
		target.Host, target.User, target.Port = strings.TrimSpace(target.Host), strings.TrimSpace(target.User), strings.TrimSpace(target.Port)
		target.Directory = strings.TrimSpace(target.Directory)
		target.Resource = strings.TrimSpace(target.Resource)
		if (target.Kind == "remote" || target.Kind == "microvm" || target.Kind == "browser" || target.Kind == "desktop" || target.Kind == "e2b") && target.Credential == "" {
			target.Credential = newID()
		}
	}
	create := f.chat == nil && f.kind == "container" && f.containerMode == "image"
	source, image := strings.TrimSpace(f.source), strings.TrimSpace(f.image)
	if create {
		if target.Directory == "" {
			target.Directory = "/workspace"
		}
		if f.storageMode == "bind" && source == "" {
			f.message = locales.WorkspaceChooseMount()
			return
		}
		if f.storageMode == "volume" {
			source = ""
		}
	}
	remember := f.remember
	options := f.containerAdvanced.options()
	f.busy, f.message, f.details, f.progress = true, "", "", locales.WorkspaceConnecting()
	if create {
		f.progress = locales.WorkspaceCreating()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if create {
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	}
	f.cancel = cancel
	go func() {
		defer cancel()
		var err error
		createdID := ""
		if create {
			createdID = newID()
			target, err = workspace.CreateContainer(ctx, target, image, source, createdID, options)
		}
		if err == nil {
			var runtime workspace.Runtime
			runtime, err = app.openWorkspace(ctx, target, secret)
			if err == nil {
				_ = runtime.Close()
			}
		}
		app.win.Update(func() {
			if app.workspaceForm != f || err != nil {
				if createdID != "" {
					go workspace.CleanupContainer(target, createdID, source == "")
				}
				if app.workspaceForm != f {
					return
				}
			}
			f.busy = false
			if err != nil {
				var key *workspace.UnknownHostKey
				if errors.As(err, &key) {
					f.pendingHostKey = key.Fingerprint
					return
				}
				f.message, f.details = locales.WorkspaceConnectionFailed(), err.Error()
				return
			}
			if remember && secret != "" && target.Credential != "" {
				if err := workspace.SaveCredential(target.Credential, secret); err != nil {
					f.remember = false
					f.message = locales.WorkspaceKeyringError()
					return
				}
			}
			if secret != "" {
				if app.workspaceSecrets == nil {
					app.workspaceSecrets = make(map[string]string)
				}
				app.workspaceSecrets[target.Credential] = secret
			}
			if f.chat == nil {
				app.clearWorkspaceSelection()
				app.draftTarget = target
			}
			app.workspaceForm = nil
		})
	}()
}
