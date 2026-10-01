package verimor

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// Router mounts the telephony endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a Verimor router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the telephony routes onto g.
func (r *Router) Routes(g fiber.Router) {
	// The phone itself (webphone, own SIP login, own status, own numbers)
	// belongs to every signed-in user; the service ties each to the caller.
	g.Get("/webphone", r.guard, r.handler.Webphone)
	g.Get("/sip/credentials", r.guard, r.handler.Credentials)
	g.Get("/pbx/status", r.guard, r.handler.Status)
	g.Post("/pbx/status", r.guard, r.handler.SetStatus)
	g.Get("/pbx/stats", r.guard, r.handler.Stats)

	sip := r.need(enums.UserUpdate, enums.AgentManage)
	g.Post("/users/:id/sip", r.guard, sip, r.handler.SetCredentials)
	g.Post("/users/:id/sip/sync", r.guard, sip, r.handler.SyncCredentials)
	g.Post("/pbx/sip/sync-all", r.guard, sip, r.handler.SyncAllCredentials)
	g.Get("/pbx/sip/sync-all", r.guard, sip, r.handler.SyncAllStatus)

	calls := r.need(enums.CDRViewAll, enums.CallViewAll, enums.CDRViewOwn, enums.CallViewOwn)
	g.Get("/calls", r.guard, calls, r.handler.Calls)
	g.Get("/calls/export", r.guard, r.need(enums.CDRExport), r.handler.ExportCalls) // before /:uuid so it is not shadowed
	g.Get("/calls/:uuid/recording", r.guard, r.need(enums.CallRecordAccess), r.handler.Recording)
	g.Post("/calls/originate", r.guard, r.need(enums.CallOriginate), r.handler.Originate)
	g.Post("/calls/transfer", r.guard, r.need(enums.CallTransfer), r.handler.Transfer)

	agents := r.need(enums.AgentView, enums.CallTransfer)
	g.Get("/pbx/extensions", r.guard, agents, r.handler.Extensions)
	g.Get("/pbx/stream", r.guard, agents, r.handler.Stream)
	g.Get("/pbx/queues", r.guard, r.need(enums.QueueView, enums.CallTransfer), r.handler.Queues)
}
