//go:build itest

package itest

var testCases = []*testCase{
	{
		name: "test loop swap",
		test: testLoopSwapV2,
	},
}

var optionalTestCases = []*testCase{}
