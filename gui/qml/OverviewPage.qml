import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Preen 1.0
import "Format.js" as Format

// Live health of the machine, and how much space Clean can free right now.
// Every tile has a fixed size, so the grid never moves as numbers change.
Kirigami.ScrollablePage {
    id: page
    title: "Overview"

    property var st: null
    property var cpuHistory: []
    property var downHistory: []
    property var upHistory: []
    property real reclaimable: -1

    readonly property var root: st ? (st.disks.find(d => d.mount === "/") || st.disks[0] || null) : null
    readonly property var mem: st ? st.memory : null
    readonly property var battery: st ? st.battery : null

    function push(list, v) {
        const out = list.concat([v])
        return out.length > 60 ? out.slice(out.length - 60) : out
    }

    function pct(a, b) {
        return b > 0 ? Math.round(a / b * 100) : 0
    }

    // The same checks as the engine's health score, in words.
    function reasons() {
        if (!st)
            return []
        const r = []
        if (root && root.free < root.total * 0.2)
            r.push(root.mount + " is " + pct(root.used, root.total) + "% full")
        if (mem && mem.used > mem.total * 0.8)
            r.push("memory " + pct(mem.used, mem.total) + "% used")
        if (st.cpu.usage > 85)
            r.push("CPU busy")
        if (st.pressure.memory > 10)
            r.push("apps are waiting for memory")
        if (st.temps.length > 0 && st.temps[0].celsius > 80)
            r.push(st.temps[0].label + " at " + Math.round(st.temps[0].celsius) + " °C")
        if (battery && battery.health_percent > 0 && battery.health_percent < 60)
            r.push("battery health " + battery.health_percent + "%")
        if (st.uptime_seconds > 30 * 86400)
            r.push("up for " + Math.floor(st.uptime_seconds / 86400) + " days")
        return r
    }

    Engine {
        id: status
        onReceived: (e) => {
            if (e.type !== "status")
                return
            page.st = e.data
            page.cpuHistory = page.push(page.cpuHistory, e.data.cpu.usage)
            page.downHistory = page.push(page.downHistory, e.data.network.down_bps || 0)
            page.upHistory = page.push(page.upHistory, e.data.network.up_bps || 0)
        }
    }
    Engine {
        id: reclaim
        onReceived: (e) => {
            if (e.type === "done")
                page.reclaimable = e.bytes || 0
        }
    }

    Component.onCompleted: {
        status.run(["status", "--watch", "--interval", "2s"])
        reclaim.run(["clean", "--dry-run"])
    }

    header: QQC2.Control {
        padding: Kirigami.Units.gridUnit
        topPadding: Kirigami.Units.largeSpacing * 2
        bottomPadding: Kirigami.Units.largeSpacing * 2
        background: Rectangle {
            color: Kirigami.Theme.backgroundColor
            Kirigami.Separator { anchors { left: parent.left; right: parent.right; bottom: parent.bottom } }
        }

        contentItem: RowLayout {
            spacing: Kirigami.Units.gridUnit

            HealthRing {
                value: page.st ? page.st.health_score : 0
            }

            ColumnLayout {
                spacing: Kirigami.Units.smallSpacing / 2
                Layout.fillWidth: true
                Kirigami.Heading {
                    level: 2
                    text: !page.st ? "Reading system status…"
                        : page.st.health_score >= 85 ? "Healthy"
                        : page.st.health_score >= 65 ? "Fair" : "Needs attention"
                }
                QQC2.Label {
                    readonly property var r: page.reasons()
                    text: r.length ? r.join(" · ") : "Nothing needs your attention"
                    visible: page.st !== null
                    elide: Text.ElideRight
                    Layout.fillWidth: true
                }
                QQC2.Label {
                    text: page.st ? page.st.host + " · " + page.st.os + " · kernel " + page.st.kernel + " · up " + Format.uptime(page.st.uptime_seconds) : ""
                    color: Kirigami.Theme.disabledTextColor
                    font: Kirigami.Theme.smallFont
                    elide: Text.ElideRight
                    Layout.fillWidth: true
                }
            }

            ColumnLayout {
                spacing: Kirigami.Units.smallSpacing / 2
                Layout.alignment: Qt.AlignVCenter
                QQC2.Label {
                    Layout.alignment: Qt.AlignRight
                    text: "Can be cleaned"
                    color: Kirigami.Theme.disabledTextColor
                    font: Kirigami.Theme.smallFont
                }
                QQC2.Label {
                    Layout.alignment: Qt.AlignRight
                    text: page.reclaimable < 0 ? "Measuring…" : Format.bytes(page.reclaimable)
                    font.pointSize: Kirigami.Theme.defaultFont.pointSize * 1.6
                    font.weight: Font.Light
                    font.features: { "tnum": 1 }
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
    }

    GridLayout {
        id: grid
        columns: width > Kirigami.Units.gridUnit * 48 ? 3 : width > Kirigami.Units.gridUnit * 28 ? 2 : 1
        columnSpacing: Kirigami.Units.largeSpacing
        rowSpacing: Kirigami.Units.largeSpacing
        opacity: page.st ? 1 : 0
        Behavior on opacity { NumberAnimation { duration: Kirigami.Units.longDuration } }

        Tile {
            title: "CPU"
            iconName: "cpu"
            value: page.st ? Math.round(page.st.cpu.usage) + "%" : ""
            detail: page.st ? "Load " + page.st.cpu.load[0].toFixed(2) + " · " + page.st.cpu.cores.length + " threads" : ""
            RowLayout {
                spacing: Kirigami.Units.largeSpacing
                Layout.fillWidth: true
                Layout.fillHeight: true
                Sparkline {
                    values: page.cpuHistory
                    maxValue: 100
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                }
                Row { // one thin bar per core
                    spacing: 2
                    Layout.fillHeight: true
                    Repeater {
                        model: page.st ? page.st.cpu.cores : []
                        delegate: Item {
                            required property real modelData
                            width: 4
                            height: parent.height
                            Rectangle {
                                anchors.fill: parent
                                radius: 1
                                color: Qt.rgba(Kirigami.Theme.textColor.r, Kirigami.Theme.textColor.g, Kirigami.Theme.textColor.b, 0.08)
                            }
                            Rectangle {
                                anchors.bottom: parent.bottom
                                width: parent.width
                                height: Math.max(1, parent.height * modelData / 100)
                                radius: 1
                                color: Kirigami.Theme.highlightColor
                                Behavior on height { NumberAnimation { duration: Kirigami.Units.longDuration } }
                            }
                        }
                    }
                }
            }
        }

        Tile {
            title: "Memory"
            iconName: "memory"
            value: page.mem ? page.pct(page.mem.used, page.mem.total) + "%" : ""
            detail: page.mem ? Format.bytes(page.mem.used) + " of " + Format.bytes(page.mem.total) + " · " + Format.bytes(page.mem.available) + " available" : ""
            Item { Layout.fillHeight: true }
            SizeBar {
                Layout.fillWidth: true
                value: page.mem ? page.mem.used / page.mem.total : 0
                barColor: page.mem && page.mem.used > page.mem.total * 0.9 ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.highlightColor
            }
            RowLayout {
                Layout.fillWidth: true
                QQC2.Label { text: "Swap"; color: Kirigami.Theme.disabledTextColor; font: Kirigami.Theme.smallFont }
                SizeBar {
                    Layout.fillWidth: true
                    value: page.mem && page.mem.swap_total > 0 ? page.mem.swap_used / page.mem.swap_total : 0
                }
                QQC2.Label {
                    text: page.mem && page.mem.swap_total > 0 ? Format.bytes(page.mem.swap_used) : "off"
                    color: Kirigami.Theme.disabledTextColor
                    font: Kirigami.Theme.smallFont
                }
            }
        }

        Tile {
            title: "Network" + (page.st && page.st.network.iface ? " · " + page.st.network.iface : "")
            iconName: page.st && page.st.network.iface.startsWith("wl") ? "network-wireless" : "network-wired"
            value: page.st ? "↓ " + Format.rate(page.st.network.down_bps || 0) : ""
            detail: page.st ? "↑ " + Format.rate(page.st.network.up_bps || 0) : ""
            Item {
                Layout.fillWidth: true
                Layout.fillHeight: true
                readonly property real peak: Math.max(1, ...page.downHistory, ...page.upHistory)
                Sparkline { anchors.fill: parent; values: page.downHistory; maxValue: parent.peak }
                Sparkline {
                    anchors.fill: parent
                    values: page.upHistory
                    maxValue: parent.peak
                    lineColor: Kirigami.Theme.disabledTextColor
                }
            }
        }

        Tile {
            title: "Disks"
            iconName: "drive-harddisk"
            value: page.root ? Format.bytes(page.root.free) + " free" : ""
            detail: page.st ? "Read " + Format.rate(page.st.disk_io.read_bps || 0) + " · write " + Format.rate(page.st.disk_io.write_bps || 0) : ""
            Repeater {
                model: page.st ? page.st.disks.slice(0, 3) : []
                delegate: RowLayout {
                    required property var modelData
                    readonly property bool low: modelData.free < modelData.total * 0.1
                    Layout.fillWidth: true
                    QQC2.Label {
                        text: modelData.mount
                        elide: Text.ElideMiddle
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 3.5
                    }
                    SizeBar {
                        Layout.fillWidth: true
                        value: modelData.used / modelData.total
                        barColor: low ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.highlightColor
                    }
                    QQC2.Label {
                        text: page.pct(modelData.used, modelData.total) + "%"
                        color: low ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.disabledTextColor
                        font.features: { "tnum": 1 }
                        horizontalAlignment: Text.AlignRight
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 2
                    }
                }
            }
        }

        Tile {
            title: "Sensors"
            iconName: "temperature-normal"
            value: page.st && page.st.temps.length ? Math.round(page.st.temps[0].celsius) + " °C" : "No sensors"
            detail: page.st && page.st.temps.length ? "hottest: " + page.st.temps[0].label : ""
            Repeater {
                model: page.st ? page.st.temps.slice(0, 3) : []
                delegate: RowLayout {
                    required property var modelData
                    readonly property color tone: modelData.celsius > 90 ? Kirigami.Theme.negativeTextColor
                                                : modelData.celsius > 80 ? Kirigami.Theme.neutralTextColor : Kirigami.Theme.highlightColor
                    Layout.fillWidth: true
                    QQC2.Label {
                        text: modelData.label
                        elide: Text.ElideRight
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 3.5
                    }
                    SizeBar { Layout.fillWidth: true; value: modelData.celsius / 100; barColor: parent.tone }
                    QQC2.Label {
                        text: Math.round(modelData.celsius) + "°"
                        color: Kirigami.Theme.disabledTextColor
                        font.features: { "tnum": 1 }
                        horizontalAlignment: Text.AlignRight
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 2
                    }
                }
            }
        }

        Tile {
            title: "Battery"
            iconName: "battery"
            value: page.battery ? page.battery.percent + "%" : "No battery"
            detail: page.battery ? page.battery.status + (page.battery.cycles ? " · " + page.battery.cycles + " cycles" : "") : "Running on mains power"
            Item { Layout.fillHeight: true }
            SizeBar {
                visible: page.battery !== null && page.battery !== undefined
                Layout.fillWidth: true
                value: page.battery ? page.battery.percent / 100 : 0
                barColor: page.battery && page.battery.percent < 15 ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.positiveTextColor
            }
            QQC2.Label {
                visible: page.battery && page.battery.health_percent > 0
                text: page.battery ? "Holds " + page.battery.health_percent + "% of its design capacity" : ""
                color: page.battery && page.battery.health_percent < 60 ? Kirigami.Theme.neutralTextColor : Kirigami.Theme.disabledTextColor
                font: Kirigami.Theme.smallFont
                elide: Text.ElideRight
                Layout.fillWidth: true
            }
        }

        Tile {
            title: "Busiest processes"
            iconName: "view-process-system"
            span: grid.columns
            rows: 1.15
            Repeater {
                model: 6 // always six rows, so the tile keeps its height
                delegate: RowLayout {
                    required property int index
                    readonly property var p: page.st && index < page.st.top_processes.length ? page.st.top_processes[index] : null
                    Layout.fillWidth: true
                    opacity: p ? 1 : 0
                    QQC2.Label {
                        text: p ? p.name : ""
                        elide: Text.ElideRight
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 8
                    }
                    SizeBar {
                        Layout.fillWidth: true
                        value: p ? Math.min(1, p.cpu / 100) : 0
                    }
                    QQC2.Label {
                        text: p ? p.cpu.toFixed(1) + "%" : ""
                        horizontalAlignment: Text.AlignRight
                        font.features: { "tnum": 1 }
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 3
                    }
                    QQC2.Label {
                        text: p ? Format.bytes(p.rss) : ""
                        color: Kirigami.Theme.disabledTextColor
                        horizontalAlignment: Text.AlignRight
                        font.features: { "tnum": 1 }
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 4.5
                    }
                }
            }
        }
    }
}
