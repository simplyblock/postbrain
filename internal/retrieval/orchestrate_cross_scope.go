package retrieval

import "context"

// OrchestrateCrossScopeContext runs retrieval for one explicit scope anchor
// with strict-scope memory behavior. It is additive and does not alter
// OrchestrateRecall defaults.
func OrchestrateCrossScopeContext(ctx context.Context, deps OrchestrateDeps, input OrchestrateInput) ([]*Result, error) {
	if input.ActiveLayers == nil {
		input.ActiveLayers = map[Layer]bool{
			LayerMemory:    true,
			LayerKnowledge: true,
			LayerSkill:     false,
		}
	}
	input.StrictScope = true
	return OrchestrateRecall(ctx, deps, input)
}
