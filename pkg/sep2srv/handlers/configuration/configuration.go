package configuration

import (
	"fmt"
	"net/http"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2srv/handlers/singleton"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store/memory"
)

// HandleConfiguration returns a singleton GET/PUT handler for /edev/{id}/cfg.
func HandleConfiguration(cfgStore *memory.ScopedStore[sep2.Configuration]) http.HandlerFunc {
	return singleton.HandleSingletonGetPut[sep2.Configuration](cfgStore,
		func(r *http.Request) string { return r.PathValue("id") },
		func(r *http.Request) sep2.Configuration {
			c := sep2.Configuration{}
			c.Href = fmt.Sprintf("/edev/%s/cfg", r.PathValue("id"))
			return c
		})
}

// Ensure store.Copier constraint is satisfied
var _ store.Copier[sep2.Configuration] = sep2.Configuration{}
