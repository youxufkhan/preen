import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Preen 1.0
import "Format.js" as Format

// Every run that changed something, newest first (from ~/.local/state/preen/operations.log).
Kirigami.ScrollablePage {
    id: page
    title: "History"

    property var runs: []
    property var collected: []
    property bool loading: true

    readonly property var icons: ({ clean: "edit-clear-all-symbolic", uninstall: "archive-remove-symbolic", purge: "folder-development-symbolic",
                                    installer: "download-symbolic", optimize: "tools-symbolic", trash: "edit-delete-symbolic" })
    readonly property var names: ({ clean: "Clean", uninstall: "Uninstall", purge: "Projects", installer: "Installers",
                                    optimize: "Maintenance", trash: "Moved to Trash" })

    Engine {
        id: engine
        onReceived: (e) => {
            if (e.type === "item")
                page.collected.push(e.item)
            else if (e.type === "done") {
                page.runs = page.collected
                page.loading = false
            }
        }
        onFinished: page.loading = false
    }
    Component.onCompleted: engine.run(["history"])

    header: Hero {
        visible: page.loading || page.runs.length > 0
        busy: page.loading
        value: page.loading ? "" : Format.bytes(page.runs.reduce((s, r) => s + (r.bytes || 0), 0))
        caption: page.loading ? "Reading history…" : "freed in total · " + Format.plural(page.runs.length, "run")
    }

    ListView {
        id: list
        currentIndex: -1
        onCountChanged: currentIndex = -1
        model: page.runs
        delegate: QQC2.ItemDelegate {
            id: row
            required property var modelData
            width: ListView.view.width
            contentItem: RowLayout {
                spacing: Kirigami.Units.largeSpacing
                Kirigami.Icon {
                    source: page.icons[row.modelData.group] || "view-history"
                    implicitWidth: Kirigami.Units.iconSizes.smallMedium
                    implicitHeight: Kirigami.Units.iconSizes.smallMedium
                }
                ColumnLayout {
                    spacing: 0
                    Layout.fillWidth: true
                    QQC2.Label { text: page.names[row.modelData.group] || row.modelData.group; Layout.fillWidth: true }
                    QQC2.Label {
                        text: row.modelData.label + " · " + row.modelData.note
                        color: Kirigami.Theme.disabledTextColor
                        font: Kirigami.Theme.smallFont
                        Layout.fillWidth: true
                    }
                }
                QQC2.Label { text: Format.bytes(row.modelData.bytes); font.features: { "tnum": 1 } }
            }
        }
        Kirigami.PlaceholderMessage {
            anchors.centerIn: parent
            width: parent.width - Kirigami.Units.gridUnit * 4
            visible: !page.loading && list.count === 0
            icon.name: "view-history"
            text: "Nothing removed yet"
            explanation: "Everything Preen removes is listed here."
        }
    }
}
