import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Preen 1.0
import "Format.js" as Format

// Installed apps from every source. Choosing one opens its removal plan in the next column.
Kirigami.ScrollablePage {
    id: page
    title: "Uninstall"

    property var apps: []
    property string openedId: ""
    property bool loading: true
    readonly property var sorts: [
        { text: "Name", by: (a, b) => a.label.localeCompare(b.label) },
        { text: "Size", by: (a, b) => (b.bytes || 0) - (a.bytes || 0) },
        { text: "Least used", by: (a, b) => (a.last_used || 0) - (b.last_used || 0) },
        { text: "Recently installed", by: (a, b) => (b.installed || 0) - (a.installed || 0) }
    ]
    readonly property var shown: {
        const q = search.text.trim().toLowerCase()
        const list = q === "" ? apps.slice() : apps.filter(a => a.label.toLowerCase().includes(q) || a.id.toLowerCase().includes(q))
        return list.sort(sorts[sortBox.currentIndex].by)
    }

    function details(a) {
        const parts = [a.group]
        if (a.note)
            parts.push(a.note)
        else {
            const used = Format.ago(a.last_used)
            const inst = Format.ago(a.installed)
            if (a.last_used && a.installed && a.last_used - a.installed < 86400)
                parts.push("not opened since it was installed or updated " + inst)
            else {
                parts.push(used ? "used " + used : "no use recorded")
                if (inst)
                    parts.push("installed or updated " + inst)
            }
        }
        return parts.join(" · ")
    }

    function load() {
        loading = true
        page.apps = []
        collected = []
        engine.run(["uninstall", "--list"])
    }
    property var collected: []

    Engine {
        id: engine
        onReceived: (e) => {
            if (e.type === "item")
                page.collected.push(e.item)
            else if (e.type === "done") {
                page.apps = page.collected
                page.loading = false
            }
        }
        onFinished: (code, err) => page.loading = false
    }

    Component.onCompleted: load()

    actions: [
        Kirigami.Action { text: "Refresh"; icon.name: "view-refresh"; enabled: !page.loading; onTriggered: page.load() }
    ]

    header: QQC2.Control {
        padding: Kirigami.Units.largeSpacing
        background: Rectangle {
            color: Kirigami.Theme.backgroundColor
            Kirigami.Separator { anchors { left: parent.left; right: parent.right; bottom: parent.bottom } }
        }
        contentItem: RowLayout {
            spacing: Kirigami.Units.smallSpacing
            Kirigami.SearchField {
                id: search
                placeholderText: page.loading ? "Finding installed apps…" : "Search " + Format.plural(page.apps.length, "app")
                Layout.fillWidth: true
            }
            QQC2.Label {
                text: "Sort by"
                color: Kirigami.Theme.disabledTextColor
            }
            QQC2.ComboBox {
                id: sortBox
                model: page.sorts.map(s => s.text)
                Layout.preferredWidth: Kirigami.Units.gridUnit * 10
            }
        }
    }

    ListView {
        id: list
        currentIndex: -1
        onCountChanged: currentIndex = -1
        model: page.shown

        delegate: QQC2.ItemDelegate {
            id: row
            required property var modelData
            required property int index
            width: ListView.view.width
            highlighted: modelData.id === page.openedId
            enabled: !modelData.disabled
            onClicked: {
                page.openedId = modelData.id
                pageStack.pop(page)
                const plan = pageStack.push(Qt.resolvedUrl("ScanPage.qml"), {
                    title: modelData.label,
                    command: "uninstall",
                    extraArgs: [modelData.id],
                    applyText: "Uninstall",
                    trash: true,
                    heroCaption: "to remove",
                    scanningText: "Finding " + modelData.label + " and its leftovers…",
                    emptyText: "Nothing to remove"
                })
                plan.applied.connect(page.load)
            }

            contentItem: RowLayout {
                spacing: Kirigami.Units.largeSpacing
                Kirigami.Icon {
                    source: row.modelData.icon || "application-x-executable"
                    fallback: "application-x-executable"
                    implicitWidth: Kirigami.Units.iconSizes.medium
                    implicitHeight: Kirigami.Units.iconSizes.medium
                }
                ColumnLayout {
                    spacing: 0
                    Layout.fillWidth: true
                    QQC2.Label {
                        text: row.modelData.label
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                    QQC2.Label {
                        text: page.details(row.modelData)
                        color: Kirigami.Theme.disabledTextColor
                        font: Kirigami.Theme.smallFont
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                }
                QQC2.Label {
                    text: Format.bytes(row.modelData.bytes)
                    font.features: { "tnum": 1 }
                    color: Kirigami.Theme.disabledTextColor
                }
            }
        }

        Kirigami.LoadingPlaceholder {
            anchors.centerIn: parent
            visible: page.loading
        }
        Kirigami.PlaceholderMessage {
            anchors.centerIn: parent
            width: parent.width - Kirigami.Units.gridUnit * 4
            visible: !page.loading && list.count === 0
            icon.name: "search"
            text: page.apps.length === 0 ? "No apps found" : "No app matches “" + search.text + "”"
        }
    }
}
