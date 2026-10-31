import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Preen 1.0
import "Format.js" as Format

// Live health of the machine, and how much space Clean can free right now.
Kirigami.ScrollablePage {
    id: page
    title: "Overview"

    property var st: null
    property var cpuHistory: []
    property var downHistory: []
    property real reclaimable: -1
    property int reclaimableCount: 0

    function push(list, v) {
        const out = list.concat([v])
        return out.length > 60 ? out.slice(out.length - 60) : out
    }

    Engine {
        id: status
        onReceived: (e) => {
            if (e.type !== "status")
                return
            page.st = e.data
            page.cpuHistory = page.push(page.cpuHistory, e.data.cpu.usage)
            page.downHistory = page.push(page.downHistory, e.data.network.down_bps || 0)
        }
    }
    Engine {
        id: reclaim
        onReceived: (e) => {
            if (e.type === "done") {
                page.reclaimable = e.bytes || 0
                page.reclaimableCount = e.count || 0
            }
        }
    }

    Component.onCompleted: {
        status.run(["status", "--watch", "--interval", "2s"])
        reclaim.run(["clean", "--dry-run"])
    }

    header: Hero {
        busy: page.st === null
        value: page.st ? page.st.health_score + " / 100" : ""
        caption: page.st ? "System health · " + page.st.host + " · " + page.st.os + " · up " + Format.uptime(page.st.uptime_seconds)
                         : "Reading system status…"

        ColumnLayout {
            spacing: 0
            QQC2.Label {
                Layout.alignment: Qt.AlignRight
                text: page.reclaimable < 0 ? "Measuring reclaimable space…" : Format.bytes(page.reclaimable) + " can be cleaned"
                color: Kirigami.Theme.disabledTextColor
            }
            QQC2.Button {
                Layout.alignment: Qt.AlignRight
                text: "Clean…"
                icon.name: "edit-clear-all"
                highlighted: true
                enabled: page.reclaimable > 0
                onClicked: applicationWindow().show("clean")
            }
        }
    }

    GridLayout {
        columns: Math.max(1, Math.floor(width / (Kirigami.Units.gridUnit * 17)))
        columnSpacing: Kirigami.Units.largeSpacing
        rowSpacing: Kirigami.Units.largeSpacing
        visible: page.st !== null

        Tile {
            title: "CPU"
            value: page.st ? Math.round(page.st.cpu.usage) + "%" : ""
            detail: page.st ? "Load " + page.st.cpu.load[0].toFixed(2) + " · " + page.st.cpu.cores.length + " threads" : ""
            Sparkline {
                Layout.fillWidth: true
                values: page.cpuHistory
                maxValue: 100
            }
        }

        Tile {
            id: memTile
            readonly property var m: page.st ? page.st.memory : null
            title: "Memory"
            value: m ? Math.round(m.used / m.total * 100) + "%" : ""
            detail: m ? Format.bytes(m.used) + " of " + Format.bytes(m.total) + (m.swap_used > 0 ? " · swap " + Format.bytes(m.swap_used) : "") : ""
            SizeBar {
                Layout.fillWidth: true
                value: memTile.m ? memTile.m.used / memTile.m.total : 0
            }
        }

        Tile {
            title: "Network"
            value: page.st ? "↓ " + Format.rate(page.st.network.down_bps || 0) : ""
            detail: page.st ? "↑ " + Format.rate(page.st.network.up_bps || 0) + (page.st.network.iface ? " · " + page.st.network.iface : "") : ""
            Sparkline {
                Layout.fillWidth: true
                values: page.downHistory
            }
        }

        Tile {
            title: "Disks"
            Repeater {
                model: page.st ? page.st.disks : []
                delegate: ColumnLayout {
                    required property var modelData
                    spacing: Kirigami.Units.smallSpacing / 2
                    Layout.fillWidth: true
                    RowLayout {
                        Layout.fillWidth: true
                        QQC2.Label { text: modelData.mount; Layout.fillWidth: true; elide: Text.ElideMiddle }
                        QQC2.Label {
                            text: Format.bytes(modelData.free) + " free"
                            color: modelData.free < modelData.total * 0.1 ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.disabledTextColor
                            font.features: { "tnum": 1 }
                        }
                    }
                    SizeBar {
                        Layout.fillWidth: true
                        value: modelData.used / modelData.total
                        barColor: modelData.free < modelData.total * 0.1 ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.highlightColor
                    }
                }
            }
            QQC2.Label {
                text: page.st ? "Read " + Format.rate(page.st.disk_io.read_bps || 0) + " · write " + Format.rate(page.st.disk_io.write_bps || 0) : ""
                color: Kirigami.Theme.disabledTextColor
                font: Kirigami.Theme.smallFont
            }
        }

        Tile {
            readonly property var b: page.st ? page.st.battery : null
            visible: b !== null && b !== undefined
            title: "Battery"
            value: b ? b.percent + "%" : ""
            detail: b ? b.status + (b.health_percent ? " · health " + b.health_percent + "%" : "") + (b.cycles ? " · " + b.cycles + " cycles" : "") : ""
        }

        Tile {
            title: "Sensors"
            visible: page.st && page.st.temps.length > 0
            Repeater {
                model: page.st ? page.st.temps : []
                delegate: RowLayout {
                    required property var modelData
                    Layout.fillWidth: true
                    QQC2.Label { text: modelData.label; Layout.fillWidth: true }
                    QQC2.Label {
                        text: Math.round(modelData.celsius) + " °C"
                        color: modelData.celsius > 85 ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.textColor
                        font.features: { "tnum": 1 }
                    }
                }
            }
        }

        Tile {
            title: "Busiest processes"
            Layout.columnSpan: Math.min(2, parent.columns)
            Repeater {
                model: page.st ? page.st.top_processes : []
                delegate: RowLayout {
                    required property var modelData
                    Layout.fillWidth: true
                    QQC2.Label { text: modelData.name; Layout.fillWidth: true; elide: Text.ElideRight }
                    QQC2.Label { text: Format.bytes(modelData.rss); color: Kirigami.Theme.disabledTextColor; font.features: { "tnum": 1 } }
                    QQC2.Label {
                        text: modelData.cpu.toFixed(1) + "%"
                        horizontalAlignment: Text.AlignRight
                        font.features: { "tnum": 1 }
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 3
                    }
                }
            }
        }
    }
}
