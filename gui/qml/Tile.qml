import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami

// One dashboard tile: a small title, one figure, a detail line, then any content.
Kirigami.AbstractCard {
    id: tile

    property string title: ""
    property string value: ""
    property string detail: ""
    default property alias content: body.data

    Layout.fillWidth: true
    Layout.alignment: Qt.AlignTop

    contentItem: ColumnLayout {
        spacing: Kirigami.Units.smallSpacing

        QQC2.Label {
            text: tile.title
            color: Kirigami.Theme.disabledTextColor
            font: Kirigami.Theme.smallFont
            Layout.fillWidth: true
        }
        QQC2.Label {
            text: tile.value
            visible: text !== ""
            font.pointSize: Kirigami.Theme.defaultFont.pointSize * 1.6
            font.weight: Font.Light
            font.features: { "tnum": 1 }
            Layout.fillWidth: true
            elide: Text.ElideRight
        }
        QQC2.Label {
            text: tile.detail
            visible: text !== ""
            color: Kirigami.Theme.disabledTextColor
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }
        ColumnLayout {
            id: body
            spacing: Kirigami.Units.smallSpacing
            Layout.fillWidth: true
        }
    }
}
