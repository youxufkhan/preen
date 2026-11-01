import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Preen 1.0
import "Format.js" as Format

// Disk explorer: each folder opens in the next column, like Dolphin's column view.
Kirigami.ScrollablePage {
    id: page

    property string path: ""            // empty: overview of the usual suspects
    property var entries: []
    property var largeFiles: []
    property real diskTotal: 0
    property real diskFree: 0
    property bool partial: false
    property bool loading: true
    property string pendingTrash: ""
    property string status: ""
    property int stepDone: 0
    property int stepTotal: 0
    property real filesSeen: 0
    property real bytesSeen: 0
    readonly property real maxBytes: entries.length ? Math.max(1, entries[0].bytes) : 1

    title: path === "" ? "Disk" : path.split("/").pop() || "/"

    function load() {
        loading = true
        entries = []
        largeFiles = []
        status = "Starting"
        stepDone = 0
        stepTotal = 0
        filesSeen = 0
        bytesSeen = 0
        engine.run(path === "" ? ["analyze"] : ["analyze", path])
    }

    function open(entry) {
        if (!entry.is_dir)
            return
        pageStack.pop(page)
        pageStack.push(Qt.resolvedUrl("DiskPage.qml"), { path: entry.path })
    }

    Engine {
        id: engine
        onReceived: (e) => {
            if (e.type === "progress") {
                page.status = e.message || page.status
                page.stepDone = e.done || 0
                page.stepTotal = e.total || 0
                page.filesSeen = e.count || 0
                page.bytesSeen = e.bytes || 0
            } else if (e.type === "entry") { // folders arrive one by one as they finish
                const list = page.entries.concat([e.data])
                list.sort((a, b) => b.bytes - a.bytes)
                page.entries = list
            } else if (e.type === "analysis") {
                page.entries = (e.data.entries || []).filter(x => x.bytes > 0)
                page.largeFiles = e.data.large_files || []
                page.diskTotal = e.data.disk_total
                page.diskFree = e.data.disk_free
                page.partial = e.data.partial
                page.loading = false
            }
        }
        onFinished: (code, err) => page.loading = false
    }

    Engine {
        id: trasher
        onReceived: (e) => {
            if (e.type === "result" && e.status === "failed")
                applicationWindow().showPassiveNotification("Could not move to Trash: " + e.message)
            else if (e.type === "done" && e.count > 0) {
                applicationWindow().showPassiveNotification("Moved to Trash · " + Format.bytes(e.bytes))
                page.load()
            }
        }
    }

    Component.onCompleted: load()

    actions: [
        Kirigami.Action {
            text: "Open folder"
            icon.name: "folder-open"
            visible: page.path !== ""
            onTriggered: Qt.openUrlExternally("file://" + page.path)
        },
        Kirigami.Action { text: "Rescan"; icon.name: "view-refresh"; enabled: !page.loading; onTriggered: page.load() }
    ]

    header: Hero {
        busy: page.loading
        progress: page.stepTotal > 0 ? page.stepDone / page.stepTotal : -1
        value: page.loading ? (page.bytesSeen > 0 ? Format.bytes(page.bytesSeen) + " measured" : "")
             : page.path === "" ? (page.diskTotal > 0 ? Format.bytes(page.diskFree) + " free" : "")
             : Format.bytes(page.entries.reduce((s, x) => s + x.bytes, 0))
        caption: page.loading ? page.status + "…"
               : page.path === "" ? "of " + Format.bytes(page.diskTotal) + " on your home disk · choose a location to look inside"
               : page.path + (page.partial ? " · some folders could not be read" : "")
        detail: page.loading && page.stepTotal > 0
                ? Format.count(page.filesSeen) + " files read · " + page.stepDone + " of " + page.stepTotal
                  + (page.path === "" ? " locations" : " folders") + " done · sizes appear below as each one finishes"
                : ""
    }

    ListView {
        id: list
        currentIndex: -1
        onCountChanged: currentIndex = -1
        model: page.entries

        delegate: QQC2.ItemDelegate {
            id: row
            required property var modelData
            width: ListView.view.width
            onClicked: page.open(modelData)

            contentItem: RowLayout {
                spacing: Kirigami.Units.largeSpacing
                Kirigami.Icon {
                    source: row.modelData.is_dir ? "folder" : "application-octet-stream"
                    implicitWidth: Kirigami.Units.iconSizes.smallMedium
                    implicitHeight: Kirigami.Units.iconSizes.smallMedium
                }
                ColumnLayout {
                    spacing: Kirigami.Units.smallSpacing / 2
                    Layout.fillWidth: true
                    RowLayout {
                        Layout.fillWidth: true
                        QQC2.Label {
                            text: row.modelData.name
                            elide: Text.ElideMiddle
                            Layout.fillWidth: true
                        }
                        QQC2.Label {
                            text: page.path === "" ? row.modelData.path.replace(/^\/home\/[^/]+/, "~") : ""
                            visible: text !== ""
                            color: Kirigami.Theme.disabledTextColor
                            font: Kirigami.Theme.smallFont
                        }
                    }
                    SizeBar {
                        value: row.modelData.bytes / page.maxBytes
                        Layout.fillWidth: true
                    }
                }
                QQC2.Label {
                    text: Format.bytes(row.modelData.bytes)
                    horizontalAlignment: Text.AlignRight
                    font.features: { "tnum": 1 }
                    Layout.preferredWidth: Kirigami.Units.gridUnit * 5
                }
                QQC2.ToolButton {
                    icon.name: "trash-empty"
                    visible: page.path !== ""
                    display: QQC2.AbstractButton.IconOnly
                    text: "Move to Trash"
                    QQC2.ToolTip.text: text
                    QQC2.ToolTip.visible: hovered
                    QQC2.ToolTip.delay: Kirigami.Units.toolTipDelay
                    onClicked: { page.pendingTrash = row.modelData.path; confirm.open() }
                }
                Kirigami.Icon {
                    source: "go-next-symbolic"
                    opacity: row.modelData.is_dir ? 0.5 : 0
                    implicitWidth: Kirigami.Units.iconSizes.small
                    implicitHeight: Kirigami.Units.iconSizes.small
                }
            }
        }

        footer: ColumnLayout {
            width: ListView.view.width
            spacing: 0
            visible: page.largeFiles.length > 0
            Kirigami.ListSectionHeader {
                text: "Largest files in this folder"
                Layout.fillWidth: true
            }
            Repeater {
                model: page.largeFiles
                delegate: QQC2.ItemDelegate {
                    required property var modelData
                    Layout.fillWidth: true
                    onClicked: Qt.openUrlExternally("file://" + modelData.path.substring(0, modelData.path.lastIndexOf("/")))
                    contentItem: RowLayout {
                        spacing: Kirigami.Units.largeSpacing
                        Kirigami.Icon {
                            source: "application-octet-stream"
                            implicitWidth: Kirigami.Units.iconSizes.smallMedium
                            implicitHeight: Kirigami.Units.iconSizes.smallMedium
                        }
                        QQC2.Label {
                            text: modelData.path.substring(page.path.length + 1)
                            elide: Text.ElideMiddle
                            Layout.fillWidth: true
                        }
                        QQC2.Label {
                            text: Format.bytes(modelData.bytes)
                            font.features: { "tnum": 1 }
                        }
                    }
                }
            }
        }

        Kirigami.LoadingPlaceholder {
            anchors.centerIn: parent
            visible: page.loading && page.entries.length === 0
        }
        Kirigami.PlaceholderMessage {
            anchors.centerIn: parent
            visible: !page.loading && page.entries.length === 0
            icon.name: "folder"
            text: "This folder is empty"
        }
    }

    Kirigami.PromptDialog {
        id: confirm
        title: "Move to Trash?"
        subtitle: page.pendingTrash + "\nYou can restore it from the Trash until you empty it."
        standardButtons: Kirigami.Dialog.NoButton
        customFooterActions: [
            Kirigami.Action {
                text: "Move to Trash"
                icon.name: "trash-empty"
                onTriggered: { confirm.close(); trasher.run(["trash", page.pendingTrash]) }
            },
            Kirigami.Action { text: "Cancel"; icon.name: "dialog-cancel"; onTriggered: confirm.close() }
        ]
    }
}
