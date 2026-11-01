package desktop

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
)

type nativeNotificationSink struct{}

func (nativeNotificationSink) Deliver(ctx context.Context, notice desktopNotice) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if notice.Sound {
		player := sessionNotificationSound(notice.Kind).Play()
		// Closing the window stops a sound already in flight, too.
		stop := context.AfterFunc(ctx, player.Close)
		timer := time.NewTimer(350 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			stop()
			player.Close()
			return ctx.Err()
		case <-timer.C:
			stop()
			player.Close()
		}
		if err := audio.Err(); err != nil {
			// Sound failure does not prevent the desktop banner.
			if notice.Desktop {
				return errors.Join(err, showDesktopNotice(ctx, notice))
			}
			return err
		}
	}
	if notice.Desktop {
		return showDesktopNotice(ctx, notice)
	}
	return nil
}

func showDesktopNotice(ctx context.Context, notice desktopNotice) error {
	if !mygo.NotificationsSupported() {
		return errors.New("desktop notifications are unavailable in this desktop environment")
	}
	// Sounds are controlled separately and played even when foreground
	// banners are disabled. Suppress the system's extra notification sound.
	return mygo.NewNotification(mygo.NotificationOptions{
		ID: notice.ID, Group: "rcode-sessions", Title: notice.Title,
		Body: notice.Body, Silent: true,
	}).ShowContext(ctx)
}

// Short, soft tones avoid external audio files and use MyGo's native output
// on Windows, macOS and Linux.
func sessionNotificationSound(kind notificationKind) *audio.Sound {
	const rate = 24000
	const duration = .28
	samples := make([]float32, int(rate*duration))
	frequencies := [2]float64{659.25, 880}
	switch kind {
	case notificationQuestion:
		frequencies = [2]float64{587.33, 783.99}
	case notificationError:
		frequencies = [2]float64{440, 329.63}
	}
	for index := range samples {
		t := float64(index) / rate
		noteTime := math.Mod(t, duration/2)
		frequency := frequencies[min(int(t/(duration/2)), 1)]
		envelope := min(noteTime/.012, 1) * math.Exp(-noteTime*18)
		samples[index] = float32(.13 * envelope * math.Sin(2*math.Pi*frequency*t))
	}
	return audio.NewSound(rate, 1, samples)
}
