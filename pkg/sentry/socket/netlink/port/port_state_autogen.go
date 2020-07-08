// automatically generated by stateify.

package port

import (
	"gvisor.dev/gvisor/pkg/state"
)

func (x *Manager) StateTypeName() string {
	return "pkg/sentry/socket/netlink/port.Manager"
}

func (x *Manager) StateFields() []string {
	return []string{
		"ports",
	}
}

func (x *Manager) beforeSave() {}

func (x *Manager) StateSave(m state.Sink) {
	x.beforeSave()
	m.Save(0, &x.ports)
}

func (x *Manager) afterLoad() {}

func (x *Manager) StateLoad(m state.Source) {
	m.Load(0, &x.ports)
}

func init() {
	state.Register((*Manager)(nil))
}