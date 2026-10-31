#pragma once

#include <QObject>
#include <QProcess>
#include <QStringList>
#include <QVariantMap>

// Engine runs `preen <command> --json ...` and turns each NDJSON line into an
// event() signal. All cleaning logic lives in the Go engine; the GUI only draws.
class Engine : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool running READ running NOTIFY runningChanged)

public:
    explicit Engine(QObject *parent = nullptr);
    ~Engine() override;

    bool running() const;
    Q_INVOKABLE void run(const QStringList &args);
    Q_INVOKABLE void cancel();

Q_SIGNALS:
    void received(const QVariantMap &event);
    void finished(int exitCode, const QString &error);
    void runningChanged();

private:
    QProcess *m_proc = nullptr;
    QByteArray m_out;
    QByteArray m_err;
};
