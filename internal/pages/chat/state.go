package chat

import (
	"context"
	"sync/atomic"
	"time"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/background"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/pages/chat/composer"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/proxy"

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

	Metrics usageMetrics `json:"metrics,omitzero"`

	RecoveryModel  *modelChoice `json:"recovery_model,omitempty"`
	RecoveryOffset *int         `json:"recovery_offset,omitempty"`

	activeTools          map[string]bool
	reasoningOpen        *bool
	interfacePreview     []interfaceUpdate
	processPreview       string
	compressing          bool
	clarificationSurface *a2ui.Surface
	copiedUntil          time.Time
	artifactImageText    string
	artifactImages       []imageDeliverable
	browser              *localtools.BrowserSession
}

type conversation struct {
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
	sentDraft          *string
	pinned             bool
	archived           bool
	menuOpen           bool
	projectedInterface *interfaceProjection
	run                *replyRun
	runError           string
	fileVersions       *localtools.FileVersions
}

type replyRun struct {
	reply  *turn
	cancel context.CancelFunc
	access *localtools.Access
}

type history struct {
	Selected string          `json:"selected"`
	Chats    []*conversation `json:"chats"`
}

type chatApp struct {
	ctx                    context.Context
	skillsHome             string
	backend                *Chat
	settings               modelSettings
	providers              providerCatalog
	providerError          string
	form                   settingsForm
	store                  *localStore
	settingsLoadErr        string
	settingsOpen           bool
	settingsTab            settings.Tab
	extensionsForm         extensionsForm
	transferBusy           bool
	transferStatus         settingsStatus
	language               settings.LanguagePreference
	busySend               busySendMode
	systemPrompt           string
	queueEdit              *queuedMessageEdit
	win                    *mygo.Window
	desktop                desktopPreferences
	testingNotification    bool
	notificationTestStatus settingsStatus
	shortcutRecording      bool
	registeredShortcut     string
	actualShortcut         string
	shortcutError          bool
	focusComposer          bool
	previewDirectory       string
	updateCheck            updateCheck
	sidebarWidth           float32
	tasks                  *background.Tasks
	backgroundOpen         bool
	backgroundError        string
	backgroundRows         []background.Row
	backgroundRefresh      time.Time
	toolPanel              toolPreviewPanel
	history                history
	historyErr             string
	chatAction             *conversationAction
	draft                  composer.Input
	skillPicker            composerSkillPicker
	replySkills            *skillIndex
	draftAttachments       []*attachment.File
	savedDraft             savedDraft
	draftErr               string
	draftVersion           uint64
	draftAccess            localtools.Mode
	draftWorkspace         string
	accessError            string
	fullAccess             *fullAccessRequest
	loadingAttachments     bool
	pickingWorkspace       bool
	attachmentError        string
	edit                   *messageEdit
	retryReply             *turn
	unreadReplies          int
	md                     *markdown.Options
	brandIcon              *ui.Bitmap
	imagePreview           chatImagePreview
	markdownImages         map[string]*markdownImage
	markdownImageProxy     proxy.Config
	savingImage            atomic.Bool
	links                  linkPreviews
}
