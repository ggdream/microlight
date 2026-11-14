package chat

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/internal/workspace"
)

type containerAdvancedForm struct {
	open   bool
	name   string
	env    []*workspace.ContainerVariable
	ports  []*workspace.ContainerPort
	mounts []*workspace.ContainerMount
}

func (form containerAdvancedForm) options() workspace.ContainerOptions {
	options := workspace.ContainerOptions{Name: strings.TrimSpace(form.name)}
	for _, row := range form.env {
		options.Env = append(options.Env, *row)
	}
	for _, row := range form.ports {
		options.Ports = append(options.Ports, *row)
	}
	for _, row := range form.mounts {
		options.Mounts = append(options.Mounts, *row)
	}
	return options
}

func (app *chatApp) containerAdvancedFields(c *ui.Context, f *workspaceForm) {
	form := &f.containerAdvanced
	ui.Column(c).FillWidth().Border(1, border).Radius(9).Padding(10, 12).Gap(12).Children(func() {
		label := locales.WorkspaceDockerAdvanced()
		rotation := float32(0)
		if form.open {
			rotation = 90
		}
		pointerButton(c.Key("toggle"), false).Label(label).FillWidth().Height(28).Gap(6).AlignItems(ui.Center).
			Children(func() {
				ui.Icon(c, icons.SVG("ios_chevron_right")).Size(10, 10).Rotate(rotation).TextColor(muted)
				ui.Text(c, label).FontSize(13).FontWeight(650)
			}).OnClick(func() { form.open = !form.open })
		if !form.open {
			return
		}
		settings.Field(c.Key("name"), locales.WorkspaceDockerName(), "", &form.name).Disabled(f.busy).OnChange(f.changed)
		ui.Text(c.Key("env-label"), locales.WorkspaceDockerEnv()).FontSize(13).FontWeight(650)
		for index, row := range form.env {
			rowContext := c.Key(row)
			ui.Row(rowContext).FillWidth().Gap(6).AlignItems(ui.End).Children(func() {
				settings.Field(rowContext.Key("name"), locales.WorkspaceDockerEnvName(), "", &row.Name).Disabled(f.busy).OnChange(f.changed)
				settings.Field(rowContext.Key("value"), locales.WorkspaceDockerEnvValue(), "", &row.Value).Disabled(f.busy).OnChange(f.changed)
				containerRemoveRow(rowContext, f, func() { form.env = slices.Delete(form.env, index, index+1) })
			})
		}
		workspaceButton(c.Key("add-env"), locales.WorkspaceDockerAddEnv(), f.busy, false, func() {
			form.env = append(form.env, &workspace.ContainerVariable{})
			f.changed()
		})
		ui.Text(c.Key("ports-label"), locales.WorkspaceDockerPorts()).FontSize(13).FontWeight(650)
		if len(form.ports) > 0 {
			ui.Text(c.Key("port-hint"), locales.WorkspaceDockerPortHint()).FontSize(11).TextColor(muted)
		}
		for index, row := range form.ports {
			rowContext := c.Key(row)
			ui.Row(rowContext).FillWidth().Gap(6).AlignItems(ui.End).Children(func() {
				settings.Field(rowContext.Key("host"), locales.WorkspaceDockerHostPort(), "8080", &row.Host).Disabled(f.busy).OnChange(f.changed)
				settings.Field(rowContext.Key("container"), locales.WorkspaceDockerContainerPort(), "80", &row.Container).Disabled(f.busy).OnChange(f.changed)
				workspaceSelect(rowContext.Key("protocol"), locales.WorkspaceDockerProtocol(), &row.Protocol,
					[]modelOption{{id: "tcp", label: "TCP"}, {id: "udp", label: "UDP"}}, f.busy, f.changed)
				containerRemoveRow(rowContext, f, func() { form.ports = slices.Delete(form.ports, index, index+1) })
			})
		}
		workspaceButton(c.Key("add-port"), locales.WorkspaceDockerAddPort(), f.busy, false, func() {
			form.ports = append(form.ports, &workspace.ContainerPort{Protocol: "tcp"})
			f.changed()
		})
		ui.Text(c.Key("mount-label"), locales.WorkspaceDockerMounts()).FontSize(13).FontWeight(650)
		for index, row := range form.mounts {
			rowContext := c.Key(row)
			ui.Row(rowContext).FillWidth().Gap(6).AlignItems(ui.End).Children(func() {
				ui.Row(rowContext.Key("source")).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.End).Children(func() {
					settings.Field(rowContext.Key("path"), locales.WorkspaceDockerMountSource(), "", &row.Source).Disabled(f.busy).OnChange(f.changed)
					label := locales.WorkspaceBrowse()
					pointerButton(rowContext.Key("browse"), f.busy).Label(label).Tooltip(label).Size(32, 35).Shrink(0).
						AlignItems(ui.Center).Justify(ui.Center).Radius(6).Background(side).
						Children(func() { icon(c, "folder_open", muted, 15) }).
						OnClick(func() { app.pickRuntimePath(f, &row.Source, true) })
				})
				settings.Field(rowContext.Key("target"), locales.WorkspaceContainerFolder(), "/data", &row.Target).Disabled(f.busy).OnChange(f.changed)
				ui.Row(rowContext.Key("mode")).Height(35).Shrink(0).AlignItems(ui.Center).Children(func() {
					theme.Checkbox(rowContext.Key("readonly"), &row.ReadOnly, locales.WorkspaceDockerReadOnly(), f.busy)
				})
				containerRemoveRow(rowContext, f, func() { form.mounts = slices.Delete(form.mounts, index, index+1) })
			})
		}
		workspaceButton(c.Key("add-mount"), locales.WorkspaceDockerAddMount(), f.busy, false, func() {
			form.mounts = append(form.mounts, &workspace.ContainerMount{})
			f.changed()
		})
	})
}

func containerRemoveRow(c *ui.Context, f *workspaceForm, remove func()) {
	label := locales.WorkspaceDockerRemoveRow()
	pointerButton(c.Key("remove"), f.busy).Label(label).Tooltip(label).Size(32, 34).Shrink(0).Radius(6).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() { icon(c, "close", muted, 14) }).
		OnClick(func() { remove(); f.changed() })
}
