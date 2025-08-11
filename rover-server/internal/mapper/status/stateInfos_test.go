// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("AppendStateInfos", func() {
	Context("when appending new state infos", func() {
		It("appends the new state infos to the existing ones", func() {
			existingStateInfos := []api.StateInfo{
				{Message: "Existing state info"},
			}
			newStateInfos := []api.StateInfo{
				{Message: "New state info"},
			}

			result := AppendStateInfos(existingStateInfos, newStateInfos)

			Expect(result).To(HaveLen(2))
			Expect(result[0].Message).To(Equal("Existing state info"))
			Expect(result[1].Message).To(Equal("New state info"))
		})
	})

	Context("when there are no new state infos", func() {
		It("returns only the existing state infos", func() {
			existingStateInfos := []api.StateInfo{
				{Message: "Existing state info"},
			}

			result := AppendStateInfos(existingStateInfos, nil)

			Expect(result).To(HaveLen(1))
			Expect(result[0].Message).To(Equal("Existing state info"))
		})
	})
})

var _ = Describe("MapProblemsToStateInfos", func() {
	Context("when mapping problems to state infos", func() {
		It("returns correctly mapped state infos", func() {
			problems := []api.Problem{
				{Message: "Problem 1", Context: "Context 1", Cause: "Cause 1"},
				{Message: "Problem 2", Context: "Context 2", Cause: "Cause 2"},
			}
			expectedStateInfos := []api.StateInfo{
				{Message: "Problem 1", Cause: "Context 1, Cause: Cause 1"},
				{Message: "Problem 2", Cause: "Context 2, Cause: Cause 2"},
			}

			stateInfos := mapProblemsToStateInfos(problems)

			Expect(stateInfos).To(Equal(expectedStateInfos))
		})
	})

	Context("when given empty problems slice", func() {
		It("returns empty state infos", func() {
			problems := []api.Problem{}
			expectedStateInfos := []api.StateInfo{}

			stateInfos := mapProblemsToStateInfos(problems)

			Expect(stateInfos).To(Equal(expectedStateInfos))
		})
	})

	Context("when given nil problems", func() {
		It("returns empty state infos", func() {
			var problems []api.Problem
			expectedStateInfos := []api.StateInfo{}

			stateInfos := mapProblemsToStateInfos(problems)

			Expect(stateInfos).To(Equal(expectedStateInfos))
		})
	})
})
