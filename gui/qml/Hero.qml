import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami

// The one large figure per page, a caption under it, and the page's primary action on the right.
QQC2.Control {
    id: hero

    property string value: ""
    property string caption: ""
    property bool busy: false
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

        QQC2.BusyIndicator {
            visible: hero.busy
            running: hero.busy
            Layout.preferredWidth: Kirigami.Units.iconSizes.large
            Layout.preferredHeight: Kirigami.Units.iconSizes.large
        }

        ColumnLayout {
            spacing: 0
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
            QQC2.Label {
                text: hero.caption
                visible: text !== ""
                color: Kirigami.Theme.disabledTextColor
                wrapMode: Text.Wrap
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
