package route

import (
	"server/controller"

	"github.com/gin-gonic/gin"
)

func blockRoute(r *gin.Engine) {
	group := r.Group("/block")
	{

		group.POST("/transaction", controller.PackTransaction)
		group.POST("/account", controller.PackAccount)
		group.POST("/uploadproposal", controller.MultiCastProposal)
		group.POST("/witness", controller.PackTransaction)
		group.POST("/uploadleader", controller.AppendBlock)

		//group.POST("/witness_2", controller.WitnessTx)
		group.POST("/validate", controller.PackValidTx)
		group.POST("/uploadroot", controller.CollectRoot)
		group.POST("/proposalBlock", controller.GetProposalBlock)
	}
}
