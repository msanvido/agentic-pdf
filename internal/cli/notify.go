package cli

import (
	"os/exec"
	"runtime"
)

// notifyUser shows a desktop notification when an agentic PDF is ready.
// Best-effort: failures are silently ignored.
func notifyUser(title, message string) {
	switch runtime.GOOS {
	case "darwin":
		script := `display notification "` + message + `" with title "` + title + `"`
		_ = exec.Command("osascript", "-e", script).Start()
	case "windows":
		ps := `-NoProfile -NonInteractive -Command ` +
			`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null;` +
			`$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02);` +
			`$t.GetElementsByTagName("text").Item(0).AppendChild($t.CreateTextNode("` + title + `")) | Out-Null;` +
			`$t.GetElementsByTagName("text").Item(1).AppendChild($t.CreateTextNode("` + message + `")) | Out-Null;` +
			`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("agentic-pdf").Show([Windows.UI.Notifications.ToastNotification]::new($t))`
		_ = exec.Command("powershell", ps).Start()
	default:
		if p, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command(p, title, message).Start()
		}
	}
}
