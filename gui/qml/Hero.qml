import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami

// The one large figure per page, a caption under it, and the page's primary action on the right.
// While busy it shows a progress bar (determinate when progress >= 0) and a detail line.
QQC2.Control {
    id: hero

    property string value: ""
    property string caption: ""
    property string detail: ""          // second, quieter line (e.g. the location being measured)
    property bool busy: false
    property real progress: -1          // 0..1, or -1 for "working, length unknown"
    default property alias actions: actionRow.data

    padding: Kirigami.Units.gridUnit
    topPadding: Kirigami.Units.largeSpacing * 2
    bottomPadding: Kirigami.Units.largeSpacing * 2

    background: Rectangle {
        color: Kirigami.Theme.backgroundColor
        Kirigami.Separator {
            anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
        }
    }

    contentItem: RowLayout {
        spacing: Kirigami.Units.gridUnit

        ColumnLayout {
            spacing: Kirigami.Units.smallSpacing / 2
            Layout.fillWidth: true

            QQC2.Label {
                text: hero.value
                visible: text !== ""
                font.pointSize: Kirigami.Theme.defaultFont.pointSize * 2.4
                font.weight: Font.Light
                font.features: { "tnum": 1 }
                Layout.fillWidth: true
                elide: Text.ElideRight
            }
            RowLayout {
                spacing: Kirigami.Units.smallSpacing
                Layout.fillWidth: true
                QQC2.BusyIndicator {
                    visible: hero.busy && hero.value === ""
                    running: visible
                    Layout.preferredWidth: Kirigami.Units.iconSizes.smallMedium
                    Layout.preferredHeight: Kirigami.Units.iconSizes.smallMedium
                }
                QQC2.Label {
                    text: hero.caption
                    visible: text !== ""
                    color: hero.busy ? Kirigami.Theme.textColor : Kirigami.Theme.disabledTextColor
                    font.features: { "tnum": 1 }
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }
            }
            QQC2.ProgressBar {
                visible: hero.busy
                indeterminate: hero.progress < 0
                value: Math.max(0, hero.progress)
                Layout.fillWidth: true
                Layout.maximumWidth: Kirigami.Units.gridUnit * 30
                Behavior on value {
                    NumberAnimation { duration: Kirigami.Units.shortDuration }
                }
            }
            QQC2.Label {
                text: hero.detail
                visible: text !== ""
                color: Kirigami.Theme.disabledTextColor
                font: Kirigami.Theme.smallFont
                elide: Text.ElideMiddle
                Layout.fillWidth: true
            }
        }

        RowLayout {
            id: actionRow
            spacing: Kirigami.Units.smallSpacing
            Layout.alignment: Qt.AlignVCenter
        }
    }
}
