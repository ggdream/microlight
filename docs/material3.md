# Material 3 UI

Microlight uses a light, neutral Material 3 theme with black as its primary color.
The application logo and user-generated content keep their colors. Errors,
destructive actions, permission warnings and file changes retain semantic colors.

Flutter's defaults establish the component hierarchy; Microlight overrides the
palette and shapes deliberately. Elevated cards use white on a near-white page,
with subtle elevation; nested filled surfaces keep a neutral gray tone. Ordinary
action buttons use an 8-point corner radius and cards use 12 points, rather than
Flutter's default stadium-shaped buttons. Save, send and confirm actions use
black with white foregrounds. Secondary actions remain tonal, elevated or text
buttons so that the primary action stays easy to identify. Switch tracks and
radio indicators keep their functional rounded shapes.
Ordinary dialogs use a 20-point radius; selection sheets retain their separate
28-point upper corners.

Settings action buttons (test, save, add, import/export and refresh) use filled
black styling; cancellation remains a text action. Form editors use white
surfaces without shadows, a 1-point neutral outline, a darker hover outline and
a 2-point black focus outline. Labels stay above the editor. Symmetric clipping
insets and stable geometry prevent text or the caret from shifting on focus.
The chat composer keeps its separate surface and layout. Selected navigation items use a
stronger neutral container independently of the pale surfaces and hover layer.
Provider choice chips use a black selected fill, white text and a checkmark;
unselected chips use a light neutral elevated surface, distinct from white cards. This is a deliberate high-contrast
monochrome adaptation of Material 3's elevated single-select chip hierarchy.
Provider chips can be dragged onto another chip to reorder them. The drag shows
a target outline and leaves the current provider unchanged. Order is a settings
draft: Save persists it, discard restores it, and Escape cancels a drag.
The switch track is compact (38 × 22 reference points), centered inside a larger
keyboard/touch target; reducing the visual track must not reduce usability.

## Shared implementation

`pkg/material3` contains pure Go components and styling functions. No native
plugin or Flutter runtime is involved. `internal/theme` adapts these components
to application icons, callbacks and shared pop-up menus; pages own their data,
navigation and existing behavior.

| Component | Use |
| --- | --- |
| `Scheme`, `Theme` | Neutral surfaces, black primary, readable foregrounds, focus and disabled states |
| `Button` / `ButtonStyle` | Filled primary action, tonal selector, elevated ordinary action, text secondary action |
| `ChoiceChipStyle`, `ChoiceChipForeground` | Single-select provider chips with a shared state layer and contrasting selected foreground |
| `DetailField` | Secondary label above a readable, wrapping detail value |
| `LoadingOverlay` | Centered modal HUD using MyGo's Spinner; blocks background pointer and keyboard interaction |
| `Card` / `CardStyle` | Elevated content groups or flat tonal nested surfaces |
| `Menu`, `Dialog`, `FloatingAction` | Anchored menus, centered alerts, floating actions |
| `InputFrame`, `SelectFrame` | Outlined form editors and filled selectors |
| `Switch` | Animated Go-rendered track and thumb, with MyGo keyboard and accessibility behavior |
| `Text` | Material role-based typography, scaled through the viewport |
| `MarkdownTheme` | Neutral code blocks and tables with readable content separators |
| `Divider`, `EdgeDivider`, `DividerColor` | Horizontal/vertical group separators and region boundaries, independently of control outlines |

Inputs retain transparent clipping insets on both sides. These insets prevent
long text, masked passwords and selections from reaching the control edge;
they are separate from the painted outline. Keep the established caret color.

## Elevation and feedback

Use elevation sparingly: low for cards and ordinary raised buttons, medium for
menus, higher for dialogs and floating actions. `Elevate` clears prior shadows
before applying a new level. MyGo appends shadows, so repeated `.Shadow` calls
would accumulate them.

Flutter's Material source is the reference for component roles and elevation
hierarchy. Its renderer uses physical elevation; our Go renderer approximates
it with Flutter's `kElevationToShadow`: umbra, penumbra and ambient layers with
20%, 14% and 12% alpha. Short-blur contact layers define the surface edge instead
of relying on a faint wide halo. Levels 1–4 use the corresponding entries;
level 5 uses Flutter's 6-point entry. Do not copy physical-elevation numbers
directly into blur radius.

Hover uses a subtle state layer, with stronger pressed/focus feedback. A row
and its inner target must not both paint the same hover surface. Low-level
`theme.PointerButton` leaves feedback to its owning component; a standalone
`material3.Button` owns its own states. Template menu icons and labels stay dark
on a light highlight. A destructive item remains red. Use `theme.MenuSurface`
for directly constructed menus so their foreground is scoped independently of
white-on-black buttons and checkboxes elsewhere.
Shared flat pop-up menus size to their longest item, including icons and inner
padding, instead of inheriting MyGo's 180-point minimum. The viewport caps their
width so long translated labels cannot extend outside the window.
Menu rows reserve 16 points of trailing padding to keep labels clear of the
right edge; this is shared by button pop-ups and context menus.

Retain existing content grouping dividers: menu sections, chat headers, settings
navigation, file panels, lists and detail cards. Material 3 uses outlineVariant
for the line; the shared divider defaults to a 1-point line centered in a
16-point space and supports leading/trailing insets, vertical orientation and
explicit color. Compact lists can request a 1-point total space. `EdgeDivider`
draws a single regional boundary without a perimeter outline. MyGo's menu
separators use the same color through the scoped menu theme's Border role;
clearing that role would hide separator rows as well as borders.

## Integration and verification

The chat view installs the theme, including Markdown. Settings cards, model and
permission selectors, MCP and built-in tool configuration, sidebar menus,
deliverables, previews, statistics, approvals, clarification, editable messages
and A2UI controls use the shared roles. Application-owned alerts render inside
the Go UI; OS file pickers, sharing, notifications and native update UI retain
their system appearance.

Responsive structure remains based on available window size. Preserve compact
message spacing, touch targets, safe areas, centered 80%-width ordinary dialogs,
and bottom sheets for phone-size selectors. Do not shrink typography to squeeze
long content into a dialog; allow the body to scroll while actions stay visible.
Conversation details use the dialog's white surface directly, with a separate
title field and grouped metadata rather than a nested gray table. Wide layouts
pair short fields in two columns; compact layouts stack them. Dividers separate
groups, not every individual field. Long titles retain inline expand/collapse.
The conversation detail dialog's Close action uses an elevated button at rest.
Configuration import performs backend creation and transactional storage work
outside the UI callback. After confirmation, a modal loading HUD blocks the
window until installation completes; Escape does not dismiss it. Export uses
the same HUD while encoding and saving a snapshot of the saved settings in the
background. Both operations use MyGo's expiring Toast for success or failure,
without an inline settings status. Canceling a picker, confirmation, or save
dismisses any loading HUD without a result toast.

Validate pointer, touch and keyboard states with independent test data. Inspect
all settings sections, their lower content, selectors, dialogs, input and
attachment states, chat, previews and A2UI at wide and compact sizes. Run the
existing no-cgo suite and official A2UI cases; browser and simulator visual
checks are local validation rather than new CI screenshot jobs.

## References

- [Material 3 design](https://m3.material.io/)
- [Flutter Material cards](https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/material/card.dart)
- [Flutter elevated buttons](https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/material/elevated_button.dart)
- [Flutter filled buttons](https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/material/filled_button.dart)
- [Flutter text buttons](https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/material/text_button.dart)
- [Flutter shadow definitions](https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/material/shadows.dart)
- [Flutter menus](https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/material/menu_anchor.dart)
- [Flutter dividers](https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/material/divider.dart)
- [Flutter choice chips](https://api.flutter.dev/flutter/material/ChoiceChip-class.html)
