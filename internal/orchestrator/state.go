// 此文件已迁移至 internal/session/state.go
// SessionState、Phase 常量和 IDFSnapshot 现在由 session 包提供，
// 避免业务模块与 orchestrator 包之间的循环导入。
//
// 如需使用这些类型，请导入 "energyplus-agent/internal/session"。

package orchestrator
