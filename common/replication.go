package common

// ReplicationStandDownMessage is the stand-down sentinel sent by a replication
// receiver when the VM was promoted on its side: the source peer must stop
// replicating (and serving) this VM. It travels either as the body of an HTTP
// 410 (sync endpoint) or inside a stream failure message (prepare/cleanup
// endpoints), so the source matches on this exact substring.
const ReplicationStandDownMessage = "replica was promoted on this peer, stand down"
