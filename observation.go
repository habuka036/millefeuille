package millefeuille

import "github.com/habuka036/millefeuille/internal/model"

// The domain types are defined in internal/model and re-exported here as type
// aliases to preserve the public API while avoiding an import cycle between the
// root package and the internal/sqlite and internal/hash packages.

// InputObservation is the caller-supplied input to AppendObservation.
type InputObservation = model.InputObservation

// Observation is the stored and returned form of an observation.
type Observation = model.Observation

// Provenance records the origin and trust metadata of an observation.
type Provenance = model.Provenance

// Receipt is returned by a successful AppendObservation call.
type Receipt = model.Receipt

// Filter specifies constraints for QueryObservations.
type Filter = model.Filter

// Page is the result type for QueryObservations.
type Page = model.Page
