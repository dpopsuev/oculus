package arch

// Backward-compatible type aliases. These exist during the strangler fig
// transition: types moved to the root oculus package, but existing consumers
// can still use arch.ContextReport etc. Remove once all consumers migrate.

import oculus "github.com/dpopsuev/oculus"

type ArchService = oculus.ArchService
type ArchEdge = oculus.ArchEdge
type ArchForbidden = oculus.ArchForbidden
type ArchModel = oculus.ArchModel
type HotSpot = oculus.HotSpot
type APISurface = oculus.APISurface
type BoundaryCrossing = oculus.BoundaryCrossing
type ScanCore = oculus.ScanCore
type GraphMetrics = oculus.GraphMetrics
type GitContext = oculus.GitContext
type DeepContext = oculus.DeepContext
type ContextReport = oculus.ContextReport
type ArchDrift = oculus.ArchDrift
