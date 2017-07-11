package rdb

import (
	cModel "github.com/Cepave/open-falcon-backend/common/model"
	rt "github.com/Cepave/open-falcon-backend/common/reflect/types"
	"github.com/Cepave/open-falcon-backend/common/utils"
	"github.com/Cepave/open-falcon-backend/modules/nqm-mng/model"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tests ListHosts(...)", ginkgoDb.NeedDb(func() {

	BeforeEach(func() {
		inTx(
			`INSERT INTO host(id, hostname)
				VALUES
					(1, 'listhosts-hostname-1'),
					(2, 'listhosts-hostname-2'),
					(3, 'listhosts-hostname-3'),
					(4, 'listhosts-hostname-4')`,
			`INSERT INTO grp(id, grp_name)
				VALUES
					(1, 'listhosts-grpname-1'),
					(2, 'listhosts-grpname-2')`,
			`INSERT INTO grp_host(grp_id, host_id)
				VALUES
					(1, 1),
					(1, 2),
					(2, 2),
					(2, 3)`,
		)
	})
	AfterEach(func() {
		inTx(
			`DELETE FROM host WHERE hostname LIKE 'listhosts-hostname-%'`,
			`DELETE FROM grp WHERE grp_name LIKE 'listhosts-grpname-%'`,
			`DELETE FROM grp_host WHERE grp_id <= 10`,
		)
	})

	DescribeTable("ListHosts(<paging>)",
		func(pageSize int, expectedHostIDs []int, expectedGroupIDs []string, expectedTotalCount int) {
			page := cModel.Paging{
				Size: int32(pageSize),
			}
			res, paging := ListHosts(page)

			resHostIDs := utils.MakeAbstractArray(res).
				MapTo(
					func(elem interface{}) interface{} {
						return elem.(*model.HostsResult).ID
					},
					rt.TypeOfInt,
				).GetArray()
			resGroupIDs := utils.MakeAbstractArray(res).
				MapTo(
					func(elem interface{}) interface{} {
						return elem.(*model.HostsResult).IdsOfGroups
					},
					rt.TypeOfString,
				).GetArray()

			Expect(resHostIDs).To(Equal(expectedHostIDs))
			Expect(resGroupIDs).To(Equal(expectedGroupIDs))
			Expect(paging.TotalCount).To(Equal(int32(expectedTotalCount)))
		},
		Entry("All all of the data", 5, []int{1, 2, 3, 4}, []string{"1", "1,2", "2", ""}, 4),
		Entry("List 1st paging of data", 2, []int{1, 2}, []string{"1", "1,2"}, 4),
	)
}))
