package memory

import (
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2"
	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
)

// Compile-time proof that the in-memory implementations satisfy the store
// contract. Without these, the interfaces in pkg/store would be aspirational:
// nothing would fail to build if an implementation drifted off them.
//
// The assertions are instantiated with real sep2 resource types rather than a
// stand-in, so they also prove the generic constraints hold for types the
// server actually stores.
var (
	_ store.ResourceReader[sep2.EndDevice] = (*Store[sep2.EndDevice])(nil)
	_ store.ResourceStore[sep2.EndDevice]  = (*Store[sep2.EndDevice])(nil)

	_ store.ScopedReader[sep2.DERStatus] = (*ScopedStore[sep2.DERStatus])(nil)
	_ store.ScopedStore[sep2.DERStatus]  = (*ScopedStore[sep2.DERStatus])(nil)

	// DERProgramStore embeds *ScopedStore and shadows some of its methods for
	// persistence, so it needs its own assertion: the embedding does not
	// guarantee the wrapper still satisfies the contract.
	_ store.ScopedStore[sep2.DERProgram] = (*DERProgramStore)(nil)
)
