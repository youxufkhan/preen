#include "engine.h"

#include <QApplication>
#include <QFileInfo>
#include <QIcon>
#include <QJsonDocument>
#include <QJsonObject>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QStandardPaths>
#include <QTimer>

// The engine binary is installed next to preen-gui; fall back to $PATH.
static QString preenPath()
{
    const QString local = QCoreApplication::applicationDirPath() + QStringLiteral("/preen");
    if (QFileInfo(local).isExecutable())
        return local;
    const QString found = QStandardPaths::findExecutable(QStringLiteral("preen"));
    return found.isEmpty() ? QStringLiteral("preen") : found;
}

Engine::Engine(QObject *parent)
    : QObject(parent)
{
}

Engine::~Engine()
{
    cancel();
}

bool Engine::running() const
{
    return m_proc != nullptr;
}

void Engine::run(const QStringList &args)
{
    cancel();
    m_out.clear();
    m_err.clear();
    auto *p = new QProcess(this);
    m_proc = p;

    QStringList full = args;
    full.insert(args.isEmpty() ? 0 : 1, QStringLiteral("--json")); // flags go before positional arguments

    connect(p, &QProcess::readyReadStandardOutput, this, [this, p] {
        m_out += p->readAllStandardOutput();
        qsizetype nl;
        while ((nl = m_out.indexOf('\n')) >= 0) {
            const QJsonDocument doc = QJsonDocument::fromJson(m_out.left(nl));
            m_out.remove(0, nl + 1);
            if (doc.isObject())
                Q_EMIT received(doc.object().toVariantMap());
        }
    });
    connect(p, &QProcess::readyReadStandardError, this, [this, p] {
        m_err += p->readAllStandardError();
    });
    connect(p, &QProcess::finished, this, [this, p](int code, QProcess::ExitStatus status) {
        const QString err = QString::fromUtf8(m_err).trimmed().section(QLatin1Char('\n'), -1).remove(QStringLiteral("preen: "));
        m_proc = nullptr;
        p->deleteLater();
        Q_EMIT runningChanged();
        Q_EMIT finished(status == QProcess::NormalExit ? code : -1, err);
    });
    connect(p, &QProcess::errorOccurred, this, [this, p](QProcess::ProcessError e) {
        if (e != QProcess::FailedToStart)
            return;
        m_proc = nullptr;
        p->deleteLater();
        Q_EMIT runningChanged();
        Q_EMIT finished(-1, QStringLiteral("Could not start the preen engine (%1)").arg(preenPath()));
    });
    p->start(preenPath(), full);
    Q_EMIT runningChanged();
}

void Engine::cancel()
{
    if (!m_proc)
        return;
    QProcess *p = m_proc;
    m_proc = nullptr;
    p->disconnect(this);
    p->terminate();
    if (!p->waitForFinished(1500))
        p->kill();
    p->deleteLater();
    Q_EMIT runningChanged();
}

int main(int argc, char *argv[])
{
    QApplication app(argc, argv); // QApplication, not QGuiApplication: the KDE desktop style draws with QStyle
    app.setApplicationName(QStringLiteral("preen"));
    app.setApplicationDisplayName(QStringLiteral("Preen"));
    app.setDesktopFileName(QStringLiteral("preen"));
    app.setApplicationVersion(QStringLiteral(PREEN_VERSION));
    app.setWindowIcon(QIcon::fromTheme(QStringLiteral("preen"), QIcon(QStringLiteral(":/preen.svg"))));
    if (qEnvironmentVariableIsEmpty("QT_QUICK_CONTROLS_STYLE"))
        QQuickStyle::setStyle(QStringLiteral("org.kde.desktop"));

    qmlRegisterType<Engine>("Preen", 1, 0, "Engine");
    QQmlApplicationEngine qml;
    qml.rootContext()->setContextProperty(QStringLiteral("startPage"), qEnvironmentVariable("PREEN_PAGE", QStringLiteral("overview")));
    qml.load(QUrl(QStringLiteral("qrc:/qml/Main.qml")));
    if (qml.rootObjects().isEmpty())
        return 1;

    // Development aid: PREEN_SCREENSHOT=out.png renders the window after a delay and exits.
    const QString shot = qEnvironmentVariable("PREEN_SCREENSHOT");
    if (!shot.isEmpty()) {
        auto *win = qobject_cast<QQuickWindow *>(qml.rootObjects().constFirst());
        const int delay = qEnvironmentVariableIsSet("PREEN_SCREENSHOT_DELAY") ? qEnvironmentVariableIntValue("PREEN_SCREENSHOT_DELAY") : 5000;
        QTimer::singleShot(delay, &app, [win, shot] {
            win->grabWindow().save(shot);
            QCoreApplication::quit();
        });
    }
    return app.exec();
}
