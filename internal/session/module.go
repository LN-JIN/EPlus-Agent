// PhaseModule 接口定义
// 所有业务阶段模块（Phase 1-6）均实现此接口，由 Orchestrator 统一驱动。
// 使用独立包避免业务模块与 orchestrator 的循环导入。

package session

import "context"

// PhaseModule 单个流程阶段的执行接口
type PhaseModule interface {
	// Name 返回阶段标识名（用于日志和进度展示），如 "intent_collection"
	Name() string

	// Run 执行阶段逻辑，通过 state 读取前置数据、写入本阶段产物
	// 返回 error 时，由 Orchestrator 决定是终止还是降级处理
	Run(ctx context.Context, state *SessionState) error
}
