package bot

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
)

// userError carries a message meant for the user ("доступ отозван", "кнопка устарела").
// replyServiceError shows it as is, unlike internal errors.
type userError struct{ msg string }

func (e userError) Error() string { return e.msg }

func userErrorf(format string, a ...any) error { return userError{msg: fmt.Sprintf(format, a...)} }

func newIncidentCode() string { return fmt.Sprintf("%06X", rand.Intn(1<<24)) }

// replyServiceError reports a failed operation of a backing service (wg-easy, telemt, ...).
// action is the failure in the user's words: "Не удалось получить список прокси".
// Admins see the details; other users get a notice naming the service and an incident code,
// so hosts, paths and API messages stay out of their chat. Details always go to the log.
func replyServiceError(ctx *Ctx, service, action string, err error) {
	var ue userError
	if errors.As(err, &ue) {
		reply(ctx.Bot, ctx.ChatID, action+": "+ue.msg)
		return
	}

	code := newIncidentCode()
	detail := fmt.Sprintf("%s: %s [%s]: %v", service, action, code, err)
	log.Printf("service error: user=%s tg_id=%d %s", ctx.User.Name, ctx.TgID, detail)
	if gLogger != nil && getSettingsCached().LogErrorsEnabled {
		gLogger.Append(formatLogLine("SVC_ERR", ctx.ChatID, ctx.User.Name, detail))
	}

	if ctx.User.Has(RoleAdmin) {
		reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("%s (%s, код %s):\n%s", action, service, code, truncate(err.Error(), 3500)))
		return
	}
	reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("⚠️ %s (%s). Попробуйте позже.\nЕсли ошибка повторяется, напишите в /support и укажите код %s.", action, service, code))
}
