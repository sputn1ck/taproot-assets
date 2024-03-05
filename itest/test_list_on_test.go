//go:build itest

package itest

var testCases = []*testCase{
	{
		name: "test loop swap",
		test: testLoopSwap,
	},
}

var optionalTestCases = []*testCase{}
