import QtQuick
import org.kde.kirigami as Kirigami

// A thin proportional bar. One accent colour everywhere; the label says what it is.
Rectangle {
    property real value: 0 // 0..1
    property color barColor: Kirigami.Theme.highlightColor

    implicitHeight: Math.round(Kirigami.Units.smallSpacing * 1.5)
    radius: height / 2
    color: Qt.rgba(Kirigami.Theme.textColor.r, Kirigami.Theme.textColor.g, Kirigami.Theme.textColor.b, 0.08)

    Rectangle {
        width: Math.max(parent.height, parent.width * Math.min(1, Math.max(0, parent.value)))
        height: parent.height
        radius: parent.radius
        color: parent.barColor
        visible: parent.value > 0
        Behavior on width {
            NumberAnimation { duration: Kirigami.Units.longDuration; easing.type: Easing.OutCubic }
        }
    }
}
