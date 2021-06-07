package ovirt

import (
	"github.com/gin-gonic/gin"
	libcnt "github.com/konveyor/controller/pkg/inventory/container"
	libmodel "github.com/konveyor/controller/pkg/inventory/model"
	libref "github.com/konveyor/controller/pkg/ref"
	api "github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1"
	model "github.com/konveyor/forklift-controller/pkg/controller/provider/model/ovirt"
	"github.com/konveyor/forklift-controller/pkg/controller/provider/web/base"
	"net/http"
)

//
// Routes.
const (
	TreeRoot   = ProviderRoot + "/tree"
	TreeVmRoot = TreeRoot + "/host"
)

//
// Types.
type Tree = base.Tree
type TreeNode = base.TreeNode

//
// Tree handler.
type TreeHandler struct {
	Handler
	// DataCenters list.
	datacenters []model.DataCenter
}

//
// Add routes to the `gin` router.
func (h *TreeHandler) AddRoutes(e *gin.Engine) {
	e.GET(TreeVmRoot, h.Tree)
}

//
// Prepare to handle the request.
func (h *TreeHandler) Prepare(ctx *gin.Context) int {
	status := h.Handler.Prepare(ctx)
	if status != http.StatusOK {
		ctx.Status(status)
		return status
	}
	db := h.Reconciler.DB()
	err := db.List(
		&h.datacenters,
		model.ListOptions{
			Detail: 1,
		})
	if err != nil {
		log.Trace(
			err,
			"url",
			ctx.Request.URL)
		return http.StatusInternalServerError
	}

	return http.StatusOK
}

//
// List not supported.
func (h TreeHandler) List(ctx *gin.Context) {
	ctx.Status(http.StatusMethodNotAllowed)
}

//
// Get not supported.
func (h TreeHandler) Get(ctx *gin.Context) {
	ctx.Status(http.StatusMethodNotAllowed)
}

//
// Tree.
func (h TreeHandler) Tree(ctx *gin.Context) {
	status := h.Prepare(ctx)
	if status != http.StatusOK {
		ctx.Status(status)
		return
	}
	if h.WatchRequest {
		ctx.Status(http.StatusBadRequest)
		return
	}
	db := h.Reconciler.DB()
	content := TreeNode{}
	for _, dc := range h.datacenters {
		tr := Tree{
			NodeBuilder: &NodeBuilder{
				provider:   h.Provider,
				reconciler: h.Reconciler,
				detail: map[string]bool{
					model.VmKind: h.Detail,
				},
			},
		}
		branch, err := tr.Build(
			&dc,
			&BranchNavigator{
				detail: h.Detail,
				db:     db,
			})
		if err != nil {
			log.Trace(
				err,
				"url",
				ctx.Request.URL)
			ctx.Status(http.StatusInternalServerError)
			return
		}
		r := DataCenter{}
		r.With(&dc)
		r.SelfLink = DataCenterHandler{}.Link(h.Provider, &dc)
		branch.Kind = model.DataCenterKind
		branch.Object = r
		content.Children = append(content.Children, branch)
	}

	ctx.JSON(http.StatusOK, content)
}

//
// Tree (branch) navigator.
type BranchNavigator struct {
	db     libmodel.DB
	detail bool
}

//
// Next (children) on the branch.
func (n *BranchNavigator) Next(p libmodel.Model) (r []model.Model, err error) {
	switch p.(type) {
	case *model.DataCenter:
		list, nErr := n.listCluster(p.(*model.DataCenter))
		if nErr == nil {
			for i := range list {
				m := &list[i]
				r = append(r, m)
			}
		} else {
			err = nErr
		}
	case *model.Cluster:
		list, nErr := n.listHost(p.(*model.Cluster))
		if nErr == nil {
			for i := range list {
				m := &list[i]
				r = append(r, m)
			}
		} else {
			err = nErr
		}
	case *model.Host:
		list, nErr := n.listVM(p.(*model.Host))
		if nErr == nil {
			for i := range list {
				m := &list[i]
				r = append(r, m)
			}
		} else {
			err = nErr
		}
	}

	return
}

func (n *BranchNavigator) listCluster(p *model.DataCenter) (list []model.Cluster, err error) {
	list = []model.Cluster{}
	err = n.db.List(
		&list,
		model.ListOptions{
			Predicate: libmodel.Eq("DataCenter", p.ID),
		})
	return
}

func (n *BranchNavigator) listHost(p *model.Cluster) (list []model.Host, err error) {
	list = []model.Host{}
	err = n.db.List(
		&list,
		model.ListOptions{
			Predicate: libmodel.Eq("Cluster", p.ID),
		})
	return
}

func (n *BranchNavigator) listVM(p *model.Host) (list []model.VM, err error) {
	detail := 0
	if n.detail {
		detail = 1
	}
	list = []model.VM{}
	err = n.db.List(
		&list,
		model.ListOptions{
			Predicate: libmodel.Eq("Host", p.ID),
			Detail:    detail,
		})
	return
}

//
// Tree node builder.
type NodeBuilder struct {
	// Data reconciler.
	reconciler libcnt.Reconciler
	// Provider.
	provider *api.Provider
	// Resource details by kind.
	detail map[string]bool
}

//
// Build a node for the model.
func (r *NodeBuilder) Node(parent *TreeNode, m model.Model) *TreeNode {
	kind := libref.ToKind(m)
	node := &TreeNode{}
	switch kind {
	case model.DataCenterKind:
		h := DataCenterHandler{}
		resource := &DataCenter{}
		resource.With(m.(*model.DataCenter))
		resource.SelfLink = h.Link(r.provider, m.(*model.DataCenter))
		object := resource.Content(r.withDetail(kind))
		node = &TreeNode{
			Parent: parent,
			Kind:   kind,
			Object: object,
		}
	case model.ClusterKind:
		h := ClusterHandler{}
		resource := &Cluster{}
		resource.With(m.(*model.Cluster))
		resource.SelfLink = h.Link(r.provider, m.(*model.Cluster))
		object := resource.Content(r.withDetail(kind))
		node = &TreeNode{
			Parent: parent,
			Kind:   kind,
			Object: object,
		}
	case model.HostKind:
		h := HostHandler{}
		resource := &Host{}
		resource.With(m.(*model.Host))
		resource.SelfLink = h.Link(r.provider, m.(*model.Host))
		object := resource.Content(r.withDetail(kind))
		node = &TreeNode{
			Parent: parent,
			Kind:   kind,
			Object: object,
		}
	case model.VmKind:
		h := VMHandler{
			Handler: Handler{
				base.Handler{
					Reconciler: r.reconciler,
					Provider:   r.provider,
				},
			},
		}
		resource := &VM{}
		h.Detail = r.withDetail(kind)
		_ = h.Build(m.(*model.VM), resource)
		object := resource.Content(h.Detail)
		node = &TreeNode{
			Parent: parent,
			Kind:   kind,
			Object: object,
		}
	case model.NetKind:
		h := NetworkHandler{}
		resource := &Network{}
		resource.With(m.(*model.Network))
		resource.SelfLink = h.Link(r.provider, m.(*model.Network))
		object := resource.Content(r.withDetail(kind))
		node = &TreeNode{
			Parent: parent,
			Kind:   kind,
			Object: object,
		}
	case model.StorageKind:
		h := StorageDomainHandler{}
		resource := &StorageDomain{}
		resource.With(m.(*model.StorageDomain))
		resource.SelfLink = h.Link(r.provider, m.(*model.StorageDomain))
		object := resource.Content(r.withDetail(kind))
		node = &TreeNode{
			Parent: parent,
			Kind:   kind,
			Object: object,
		}
	}

	return node
}

//
// Build with detail.
func (r *NodeBuilder) withDetail(kind string) bool {
	if b, found := r.detail[kind]; found {
		return b
	}

	return false
}