package chat

import (
	"context"
	"time"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/pages/chat/composer"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/internal/workspace"
	"github.com/ggdream/microlight/pkg/a2ui"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

const replyRequestFailed = "request_failed"

type turn struct {
	message
	State           string                `json:"state,omitempty"`
	Error           string                `json:"error,omitempty"`
	Activity        []timeline.Entry      `json:"activity,omitempty"`
	Reasoning       string                `json:"reasoning,omitempty"`
	StartedAt       time.Time             `json:"started_at,omitempty"`
	FinishedAt      time.Time             `json:"finished_at,omitempty"`
	Clarifications  []clarification       `json:"clarifications,omitempty"`
	Interface       []interfaceUpdate     `json:"a2ui,omitempty"`
	InterfaceAction *a2ui.RendererMessage `json:"a2ui_action,omitempty"`

	reasoningOpen        *bool
	interfacePreview     []interfaceUpdate
	processPreview       string
	compressing          bool
	clarificationSurface *a2ui.Surface
	copiedUntil          time.Time
	remoteCommands       []savedRemoteCommand
	resumeCommand        *savedRemoteCommand
	recoveringRemote     bool
	stopRemoteRecovery   context.CancelCauseFunc
}

type conversation struct {
	detachedCommands   *turn
	ID                 string  `json:"id"`
	Title              string  `json:"title"`
	Turns              []*turn `json:"turns"`
	queue              []queuedMessage
	list               ui.ListState
	sessionID          string
	sessionModel       string
	branchID           string
	accessMode         localtools.Mode
	workspace          string
	target             workspace.Target
	browser            *workspace.LocalBrowser
	browserID          string
	sentDraft          *string
	pinned             bool
	archived           bool
	menuOpen           bool
	projectedInterface *interfaceProjection
}

type history struct {
	Selected string          `json:"selected"`
	Chats    []*conversation `json:"chats"`
}

type chatApp struct {
	desktopID          string
	skillsHome         string
	backend            *Chat
	settings           modelSettings
	providers          providerCatalog
	providerError      string
	form               settingsForm
	store              *localStore
	settingsLoadErr    string
	settingsOpen       bool
	settingsTab        settings.Tab
	extensionsForm     extensionsForm
	language           settings.LanguagePreference
	busySend           busySendMode
	queueEdit          *queuedMessageEdit
	win                *mygo.Window
	sidebarWidth       float32
	toolPanel          toolPreviewPanel
	history            history
	historyErr         string
	chatAction         *conversationAction
	draft              composer.Input
	skillPicker        composerSkillPicker
	replySkills        *skillIndex
	draftAttachments   []*attachment.File
	savedDraft         savedDraft
	draftErr           string
	draftVersion       uint64
	draftAccess        localtools.Mode
	draftWorkspace     string
	draftTarget        workspace.Target
	workspaceForm      *workspaceForm
	workspaceSecrets   map[string]string
	accessError        string
	fullAccess         *fullAccessRequest
	runningAccess      *localtools.Access
	loadingAttachments bool
	pickingWorkspace   bool
	attachmentError    string
	active             *conversation
	running            *turn
	cancel             context.CancelFunc
	edit               *messageEdit
	retryReply         *turn
	unreadReplies      int
	md                 *markdown.Options
	links              linkPreviews
}
