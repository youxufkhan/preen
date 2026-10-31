import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Preen 1.0
import "Format.js" as Format

// Scan → Review → Apply, shared by Clean, Projects, Installers, Maintenance and an app's uninstall plan.
Kirigami.ScrollablePage {
    id: page

    property string command: "clean"
    property var extraArgs: []          // positional arguments, e.g. the app id for uninstall
    property string applyText: "Clean"
    property string scanningText: "Scanning…"
    property string emptyText: "Nothing found"
    property string emptyExplanation: ""
    property string heroCaption: "selected"
    property bool countMode: false      // Maintenance: count tasks, not bytes
    property bool trash: false          // Uninstall: user files go to the Trash
    property string unit: countMode ? "task" : "item"

    signal applied()

    property string phase: "scanning"   // scanning | ready | applying | done | error
    property string status: ""
    property string errorText: ""
    property real selectedBytes: 0
    property int selectedCount: 0
    property int foundCount: 0
    property real maxBytes: 1
    property bool rootSelected: false
    property real freed: 0
    property int freedCount: 0
    property int failedCount: 0
    property real shownFreed: 0
    property real freeAfter: 0

    function scan() {
        items.clear()
        errorText = ""
        freed = 0
        shownFreed = 0
        failedCount = 0
        phase = "scanning"
        status = scanningText
        engine.run([command, "--dry-run"].concat(extraArgs))
    }

    function apply() {
        let args = [command, "--yes"]
        for (let i = 0; i < items.count; i++) {
            const it = items.get(i)
            if (it.selected)
                args.push("--only", it.id)
        }
        phase = "applying"
        status = rootSelected ? "Waiting for your password…" : "Working…"
        engine.run(args.concat(extraArgs))
    }

    function setSelected(i, on) {
        items.setProperty(i, "selected", on)
        recount()
    }

    function recount() {
        let bytes = 0, n = 0, found = 0, max = 1, root = false
        for (let i = 0; i < items.count; i++) {
            const it = items.get(i)
            if (!it.disabled)
                found++
            max = Math.max(max, it.bytes)
            if (it.selected) {
                bytes += it.bytes
                n++
                root = root || it.root
            }
        }
        selectedBytes = bytes
        selectedCount = n
        foundCount = found
        maxBytes = max
        rootSelected = root
    }

    function handle(e) {
        switch (e.type) {
        case "progress":
            status = e.message
            break
        case "item": {
            if (phase !== "scanning")
                break // the apply run repeats the list; keep the rows we already show
            const it = e.item
            items.append({
                id: it.id, group: it.group, label: it.label, bytes: it.bytes || 0, risk: it.risk || "safe",
                selected: !!it.selected, disabled: !!it.disabled, root: !!it.root, note: it.note || "",
                icon: it.icon || "", entries: JSON.stringify(it.entries || []), result: "", message: ""
            })
            break
        }
        case "result":
            for (let i = 0; i < items.count; i++) {
                if (items.get(i).id === e.id) {
                    items.setProperty(i, "result", e.status)
                    items.setProperty(i, "message", e.message || "")
                    if (e.status === "failed")
                        failedCount++
                }
            }
            break
        case "done":
            if (phase === "scanning") {
                recount()
                phase = "ready"
            } else if (phase === "applying") {
                freed = e.bytes || 0
                freedCount = e.count || 0
                freeAfter = e.free || 0
                phase = "done"
                countUp.restart()
                page.applied()
            }
            break
        case "error":
            errorText = e.message
            phase = "error"
            break
        }
    }

    NumberAnimation {
        id: countUp
        target: page
        property: "shownFreed"
        from: 0
        to: page.freed
        duration: 900
        easing.type: Easing.OutCubic
    }

    Engine {
        id: engine
        onReceived: (e) => page.handle(e)
        onFinished: (code, err) => {
            if (code !== 0 && page.phase !== "done") {
                page.errorText = err || "The engine stopped unexpectedly."
                page.phase = "error"
            }
        }
    }

    ListModel { id: items }

    Component.onCompleted: scan()

    actions: [
        Kirigami.Action {
            text: "Scan again"
            icon.name: "view-refresh"
            enabled: page.phase !== "scanning" && page.phase !== "applying"
            onTriggered: page.scan()
        }
    ]

    header: ColumnLayout {
        spacing: 0

        Hero {
            Layout.fillWidth: true
            busy: page.phase === "scanning" || page.phase === "applying"
            value: {
                switch (page.phase) {
                case "scanning": return ""
                case "applying": return ""
                case "done": return page.countMode ? Format.plural(page.freedCount, "task") + " done" : "Freed " + Format.bytes(page.shownFreed)
                case "error": return ""
                default: return page.countMode ? Format.plural(page.selectedCount, "task") : Format.bytes(page.selectedBytes)
                }
            }
            caption: {
                switch (page.phase) {
                case "scanning":
                case "applying": return page.status
                case "done": return Format.plural(page.freedCount, page.unit) + " done"
                                    + (page.failedCount > 0 ? " · " + page.failedCount + " failed, see below" : "")
                                    + (page.freeAfter > 0 ? " · " + Format.bytes(page.freeAfter) + " free on /" : "")
                case "error": return ""
                default: return page.foundCount === 0 ? "" : page.heroCaption + (page.heroCaption ? " · " : "")
                                + page.selectedCount + " of " + Format.plural(page.foundCount, page.unit)
                }
            }

            QQC2.Button {
                text: "Review…"
                icon.name: "view-list-details"
                visible: page.phase === "ready" && page.foundCount > 0
                enabled: page.selectedCount > 0
                highlighted: true
                onClicked: review.open()
            }
            QQC2.Button {
                text: "Cancel"
                visible: page.phase === "scanning"
                onClicked: { engine.cancel(); page.phase = "ready"; page.recount() }
            }
        }

        Kirigami.InlineMessage {
            Layout.fillWidth: true
            Layout.margins: Kirigami.Units.smallSpacing
            visible: page.phase === "error"
            type: Kirigami.MessageType.Error
            text: page.errorText
            actions: Kirigami.Action { text: "Try again"; icon.name: "view-refresh"; onTriggered: page.scan() }
        }
    }

    ListView {
        id: list
        model: items
        reuseItems: true
        currentIndex: -1
        onCountChanged: currentIndex = -1
        section.property: "group"
        section.delegate: Kirigami.ListSectionHeader {
            required property string section
            width: ListView.view.width
            text: section
        }

        delegate: QQC2.ItemDelegate {
            id: row
            required property int index
            required property var model
            width: ListView.view.width
            enabled: page.phase === "ready" || model.result !== ""
            onClicked: if (!model.disabled) page.setSelected(index, !model.selected)

            contentItem: RowLayout {
                spacing: Kirigami.Units.largeSpacing

                QQC2.CheckBox {
                    checked: row.model.selected
                    enabled: !row.model.disabled && page.phase === "ready"
                    visible: row.model.result === "" && row.model.risk !== "info"
                    onToggled: page.setSelected(row.index, checked)
                }
                Kirigami.Icon {
                    visible: row.model.result !== ""
                    source: row.model.result === "applied" ? "emblem-ok-symbolic"
                          : row.model.result === "failed" ? "data-error" : "data-information"
                    color: row.model.result === "applied" ? Kirigami.Theme.positiveTextColor
                         : row.model.result === "failed" ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.disabledTextColor
                    isMask: true
                    implicitWidth: Kirigami.Units.iconSizes.smallMedium
                    implicitHeight: Kirigami.Units.iconSizes.smallMedium
                }
                Kirigami.Icon {
                    source: row.model.icon
                    visible: row.model.icon !== ""
                    readonly property int size: row.model.risk === "info" ? Kirigami.Units.iconSizes.smallMedium : Kirigami.Units.iconSizes.medium
                    implicitWidth: size
                    implicitHeight: size
                }

                ColumnLayout {
                    spacing: Kirigami.Units.smallSpacing / 2
                    Layout.fillWidth: true

                    QQC2.Label {
                        text: row.model.label
                        elide: Text.ElideMiddle
                        opacity: row.model.disabled && row.model.risk !== "info" ? 0.6 : 1
                        Layout.fillWidth: true
                    }
                    RowLayout {
                        spacing: Kirigami.Units.smallSpacing
                        visible: badge.visible || note.text !== ""
                        Layout.fillWidth: true
                        RiskBadge { id: badge; risk: row.model.risk }
                        QQC2.Label {
                            id: note
                            text: row.model.result === "failed" ? row.model.message : row.model.note
                            color: row.model.result === "failed" ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.disabledTextColor
                            font: Kirigami.Theme.smallFont
                            elide: Text.ElideRight
                            Layout.fillWidth: true
                        }
                    }
                    SizeBar {
                        visible: !page.countMode && row.model.bytes > 0
                        value: row.model.bytes / page.maxBytes
                        opacity: row.model.selected ? 1 : 0.35
                        Layout.fillWidth: true
                    }
                }

                QQC2.Label {
                    visible: !page.countMode
                    text: Format.bytes(row.model.bytes)
                    horizontalAlignment: Text.AlignRight
                    font.features: { "tnum": 1 }
                    opacity: row.model.selected ? 1 : 0.6
                    Layout.preferredWidth: Kirigami.Units.gridUnit * 5
                }
            }
        }

        Kirigami.PlaceholderMessage {
            anchors.centerIn: parent
            width: parent.width - Kirigami.Units.gridUnit * 4
            visible: page.phase === "ready" && items.count === 0
            icon.name: "checkmark"
            text: page.emptyText
            explanation: page.emptyExplanation
        }
    }

    Kirigami.Dialog {
        id: review
        title: "Review " + Format.plural(page.selectedCount, page.unit)
        preferredWidth: Kirigami.Units.gridUnit * 34
        preferredHeight: Kirigami.Units.gridUnit * 28
        standardButtons: Kirigami.Dialog.NoButton
        flatFooterButtons: false

        onOpened: {
            reviewModel.clear()
            for (let i = 0; i < items.count; i++) {
                const it = items.get(i)
                if (it.selected)
                    reviewModel.append({ label: it.label, bytes: it.bytes, entries: it.entries })
            }
        }

        customFooterActions: [
            Kirigami.Action {
                text: page.countMode ? page.applyText + " " + Format.plural(page.selectedCount, "task")
                                     : page.applyText + " " + Format.bytes(page.selectedBytes)
                icon.name: page.countMode ? "media-playback-start" : (page.trash ? "trash-empty" : "edit-delete")
                onTriggered: { review.close(); page.apply() }
            },
            Kirigami.Action {
                text: "Cancel"
                icon.name: "dialog-cancel"
                onTriggered: review.close()
            }
        ]

        ListView {
            id: reviewList
            model: ListModel { id: reviewModel }
            implicitHeight: contentHeight
            header: QQC2.Label {
                width: ListView.view.width
                padding: Kirigami.Units.largeSpacing
                wrapMode: Text.Wrap
                color: Kirigami.Theme.disabledTextColor
                text: (page.countMode ? "These tasks will run."
                       : page.trash ? "The app is removed. Files in your home folder go to the Trash."
                       : "These files are deleted permanently.")
                      + (page.rootSelected ? " You will be asked for your password once for system items." : "")
            }
            delegate: ColumnLayout {
                id: reviewRow
                required property var model
                readonly property var paths: JSON.parse(model.entries || "[]")
                width: ListView.view.width
                spacing: 0
                RowLayout {
                    Layout.fillWidth: true
                    Layout.leftMargin: Kirigami.Units.largeSpacing
                    Layout.rightMargin: Kirigami.Units.largeSpacing
                    Layout.topMargin: Kirigami.Units.smallSpacing
                    Kirigami.Heading {
                        level: 5
                        text: reviewRow.model.label
                        elide: Text.ElideMiddle
                        Layout.fillWidth: true
                    }
                    QQC2.Label {
                        visible: !page.countMode
                        text: Format.bytes(reviewRow.model.bytes)
                        font.features: { "tnum": 1 }
                    }
                }
                Repeater {
                    model: {
                        const all = reviewRow.paths
                        return all.length > 6 ? all.slice(0, 6).concat([{ path: "and " + (all.length - 6) + " more", bytes: 0 }]) : all
                    }
                    delegate: RowLayout {
                        required property var modelData
                        Layout.fillWidth: true
                        Layout.leftMargin: Kirigami.Units.largeSpacing * 2
                        Layout.rightMargin: Kirigami.Units.largeSpacing
                        QQC2.Label {
                            text: modelData.path
                            font: Kirigami.Theme.smallFont
                            color: Kirigami.Theme.disabledTextColor
                            elide: Text.ElideMiddle
                            Layout.fillWidth: true
                        }
                        QQC2.Label {
                            text: modelData.bytes > 0 && reviewRow.paths.length > 1 ? Format.bytes(modelData.bytes) : ""
                            font: Kirigami.Theme.smallFont
                            color: Kirigami.Theme.disabledTextColor
                        }
                    }
                }
            }
        }
    }
}
