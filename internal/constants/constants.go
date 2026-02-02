package constants

import "time"

const ControlPlaneNotReadyRequeueAfter time.Duration = 15 * time.Second

const InfrastructureNotReadyRequeueAfter time.Duration = 15 * time.Second
