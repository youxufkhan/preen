import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami

// One dashboard tile with a fixed size: an icon and title, one figure, a detail line, then content.
// Width and height never follow the content, so live numbers cannot shift the grid.
Kirigami.AbstractCard {
    id: tile

    property string title: ""
    property string iconName: ""
    property string value: ""
    property string detail: ""
    property real rows: 1          // height in tile rows
    property int span: 1           // columns to cover
    default property alias content: body.data

    Layout.fillWidth: true
    Layout.preferredWidth: 10 * span // equal shares; content never decides the width
    Layout.minimumWidth: 0
    Layout.columnSpan: span
    Layout.preferredHeight: Kirigami.Units.gridUnit * 8.5 * rows
    Layout.minimumHeight: Layout.preferredHeight
    Layout.maximumHeight: Layout.preferredHeight

    contentItem: ColumnLayout {
        spacing: Kirigami.Units.smallSpacing
        clip: true

        RowLayout {
            spacing: Kirigami.Units.smallSpacing
            Layout.fillWidth: true
            Kirigami.Icon {
                source: tile.iconName
                visible: tile.iconName !== ""
                implicitWidth: Kirigami.Units.iconSizes.small
                implicitHeight: Kirigami.Units.iconSizes.small
            }
            QQC2.Label {
                text: tile.title
                color: Kirigami.Theme.disabledTextColor
                font: Kirigami.Theme.smallFont
                elide: Text.ElideRight
                Layout.fillWidth: true
            }
        }
        QQC2.Label {
            text: tile.value
            visible: text !== ""
            font.pointSize: Kirigami.Theme.defaultFont.pointSize * 1.6
            font.weight: Font.Light
            font.features: { "tnum": 1 }
            elide: Text.ElideRight
            Layout.fillWidth: true
        }
        QQC2.Label {
            text: tile.detail
            visible: text !== ""
            color: Kirigami.Theme.disabledTextColor
            font.features: { "tnum": 1 }
            elide: Text.ElideRight
            Layout.fillWidth: true
        }
        ColumnLayout {
            id: body
            spacing: Kirigami.Units.smallSpacing
            Layout.fillWidth: true
            Layout.fillHeight: true
        }
    }
}
