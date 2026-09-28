package bus

// Inference uses request/reply on per-node subjects.
// Gateway: Request("tesla.infer.request.<nodeID>", ...)
// Agent: Subscribe("tesla.infer.request.<nodeID>")
const InferRequestPrefix = "tesla.infer.request."
