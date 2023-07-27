//go:build itest

package itest

var testCases = []*testCase{
	{
		name: "musig2spend",
		test: testMusig2Spend,
	},
}

var optionalTestCases = []*testCase{}
