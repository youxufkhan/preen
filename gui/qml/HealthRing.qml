import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami

// Health score as a ring: green from 85, amber from 65, red below.
Item {
    id: ring

    property real value: 0 // 0..100
    readonly property color tone: value >= 85 ? Kirigami.Theme.positiveTextColor
                                : value >= 65 ? Kirigami.Theme.neutralTextColor : Kirigami.Theme.negativeTextColor
    property real shown: value
    Behavior on shown {
        NumberAnimation { duration: Kirigami.Units.longDuration * 2; easing.type: Easing.OutCubic }
    }

    implicitWidth: Kirigami.Units.gridUnit * 4.5
    implicitHeight: implicitWidth

    onShownChanged: canvas.requestPaint()
    onToneChanged: canvas.requestPaint()

    Canvas {
        id: canvas
        anchors.fill: parent
        onPaint: {
            const ctx = getContext("2d")
            ctx.reset()
            const r = width / 2 - 4, c = width / 2
            ctx.lineWidth = 6
            ctx.lineCap = "round"
            ctx.strokeStyle = Qt.rgba(Kirigami.Theme.textColor.r, Kirigami.Theme.textColor.g, Kirigami.Theme.textColor.b, 0.1)
            ctx.beginPath()
            ctx.arc(c, c, r, 0, 2 * Math.PI)
            ctx.stroke()
            if (ring.shown <= 0)
                return
            ctx.strokeStyle = ring.tone
            ctx.beginPath()
            ctx.arc(c, c, r, -Math.PI / 2, -Math.PI / 2 + 2 * Math.PI * Math.min(1, ring.shown / 100))
            ctx.stroke()
        }
    }

    QQC2.Label {
        anchors.centerIn: parent
        text: Math.round(ring.shown)
        font.pointSize: Kirigami.Theme.defaultFont.pointSize * 1.8
        font.weight: Font.Light
        font.features: { "tnum": 1 }
    }
}
