package migrate

import "testing"

func TestRunsInTransactionDirective(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want bool
	}{
		{
			name: "plain migration runs in a transaction",
			sql:  "CREATE TABLE example (id int);\n",
			want: true,
		},
		{
			name: "leading comment then sql still runs in a transaction",
			sql:  "-- a note\n\nCREATE INDEX example_idx ON example (id);\n",
			want: true,
		},
		{
			name: "explicit directive opts out",
			sql:  "-- migrate:no-transaction\nCREATE INDEX CONCURRENTLY example_idx ON example (id);\n",
			want: false,
		},
		{
			name: "directive is case insensitive and may follow other comments",
			sql:  "-- header\n-- MIGRATE:NO-TRANSACTION\nCREATE INDEX CONCURRENTLY example_idx ON example (id);\n",
			want: false,
		},
		{
			name: "directive after a statement is ignored",
			sql:  "CREATE TABLE example (id int);\n-- migrate:no-transaction\n",
			want: true,
		},
		{
			name: "empty file runs in a transaction",
			sql:  "",
			want: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := runsInTransaction(testCase.sql); got != testCase.want {
				t.Fatalf("runsInTransaction() = %v, want %v", got, testCase.want)
			}
		})
	}
}
