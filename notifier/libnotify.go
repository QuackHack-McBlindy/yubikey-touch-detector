package notifier

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/esiqveland/notify"
	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
)

// SetupLibnotifyNotifier configures a notifier to show all touch requests with libnotify
func SetupLibnotifyNotifier(notifiers *sync.Map) {
	touch := make(chan Message, 10)
	notifiers.Store("notifier/libnotify", touch)

	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		log.Error("Cannot initialize desktop notifications, unable to create session bus: ", err)
		return
	}
	defer conn.Close()

	if err := conn.Auth(nil); err != nil {
		log.Error("Cannot initialize desktop notifications, unable to authenticate: ", err)
		return
	}

	if err := conn.Hello(); err != nil {
		log.Error("Cannot initialize desktop notifications, unable to get bus name: ", err)
		return
	}

	notification := notify.Notification{
		AppName: "yubikey-touch-detector",
		AppIcon: "yubikey-touch-detector",
		Summary: "YubiKey is waiting for a touch",
	}

	reset := func(msg *notify.NotificationClosedSignal) {
		atomic.CompareAndSwapUint32(&notification.ReplacesID, msg.ID, 0)
	}

	notifier, err := notify.New(
		conn,
		notify.WithOnClosed(reset),
		notify.WithLogger(log.StandardLogger()),
	)
	if err != nil {
		log.Error("Cannot initialize desktop notifications, unable to initialize D-Bus notifier interface: ", err)
		return
	}
	defer notifier.Close()

	activeTouchWaits := 0

	for {
		value := <-touch
		var message string
		switch value {
		case GPG_ON:
			activeTouchWaits++
			message = "Waiting for GPG touch..."
		case U2F_ON:
			activeTouchWaits++
			message = "Tap the device to authenticate for U2F."
		case HMAC_ON:
			activeTouchWaits++
			message = "Tap the device for HMAC authentication."
		case GPG_OFF, U2F_OFF, HMAC_OFF:
			activeTouchWaits--
			message = ""
		}

		// Show notification if touch is waiting
		if activeTouchWaits > 0 && message != "" {
			id, err := notifier.SendNotification(notification)
			if err != nil {
				log.Error("Cannot show notification: ", err)
				continue
			}
			atomic.CompareAndSwapUint32(&notification.ReplacesID, 0, id)
		} else if id := atomic.LoadUint32(&notification.ReplacesID); id != 0 {
			if _, err := notifier.CloseNotification(id); err != nil {
				log.Error("Cannot close notification: ", err)
				continue
			}
		}

		// Trigger the TTS script if a touch message is available
		if message != "" {
			go triggerTTS(message)
		}
	}
}

// Function to call the bash script for TTS
func triggerTTS(message string) {
	// Sanitize the message to prevent any issues with special characters
	sanitizedMessage := strings.TrimSpace(message)

	// Prepare the shell script call with the message
	cmd := exec.Command("/bin/bash", "/path/to/your/script.sh", sanitizedMessage)

	// Set up the environment for the TTS script
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR=/run/user/1000") // Set necessary environment variables

	// Run the script
	if err := cmd.Run(); err != nil {
		log.Error("Error triggering TTS: ", err)
	}
}
