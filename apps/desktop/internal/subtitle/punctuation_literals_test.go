package subtitle

import "testing"

// Cleanup must distinguish sentence stops from literal data in visible text,
// including literals whose presentation is interrupted by formatting tags.
func TestPunctuationPreservesLiterals(t *testing.T) {
	windowsExample := "C:" + "\\Sub\\file.srt"
	for _, tc := range []struct{ text, want string }{
		{"Email AsiaPacific@bbc.co.uk. Next.", "Email AsiaPacific@bbc.co.uk, Next,"},
		{"User first.last+news@example.org. Done.", "User first.last+news@example.org, Done,"},
		{`Email "first.last"@bbc.co.uk. Next.`, `Email "first.last"@bbc.co.uk, Next,`},
		{"Email first.last@[127.0.0.1]. Done.", "Email first.last@[127.0.0.1], Done,"},
		{"Email User.Name＠BBC．CO．UK. Done.", "Email User.Name＠BBC．CO．UK, Done,"},
		{"Email NAME@BBC.CO.UK. Done.", "Email NAME@BBC.CO.UK, Done,"},
		{"联系：用户@例子.中国。完成。", "联系：用户@例子.中国，完成，"},
		{"Email δοκιμή@παράδειγμα.δοκιμή. Done.", "Email δοκιμή@παράδειγμα.δοκιμή, Done,"},
		{"Read https://example.test/a.b?key=c.d#part.e. Next.", "Read https://example.test/a.b?key=c.d#part.e, Next,"},
		{"Visit bbc.co.uk and www.example.com. Done.", "Visit bbc.co.uk and www.example.com, Done,"},
		{"Use 3.14, 10,000.50, ３．１４ and v1.2.3. Next.", "Use 3.14, 10,000.50, ３．１４ and v1.2.3, Next,"},
		{"Use .5, -.25 and 1.5e-3. Next.", "Use .5, -.25 and 1.5e-3, Next,"},
		{"Connect 192.168.0.1 or [::ffff:192.0.2.1]. Done.", "Connect 192.168.0.1 or [::ffff:192.0.2.1], Done,"},
		{"Meet Dr. Smith and J. K. Rowling in the U.S.A. at 9 a.m. Today.", "Meet Dr. Smith and J. K. Rowling in the U.S.A. at 9 a.m. Today,"},
		{"Open README.md, package.json and .env.local. Next.", "Open README.md, package.json and .env.local, Next,"},
		{"Edit .gitignore and .env. Done.", "Edit .gitignore and .env, Done,"},
		{"Paths data/sub/ and .git/. Done.", "Paths data/sub/ and .git/, Done,"},
		{"Open " + windowsExample + " or ./output.vtt. Done.", "Open " + windowsExample + " or ./output.vtt, Done,"},
		{"Run `obj.method()` and module.submodule. Done.", "Run `obj.method()` and module.submodule, Done,"},
		{"Wait... Then…… Next… Done.", "Wait... Then…… Next… Done,"},
		{"First. Next． Again。最后。", "First, Next, Again，最后，"},
		{`<i>AsiaPacific@bbc</i>.co.uk. Done.`, `<i>AsiaPacific@bbc</i>.co.uk, Done,`},
		{`{\fs12.5}3.14。Next。\NEmail AsiaPacific@bbc.co.uk.`, `{\fs12.5}3.14，Next，\NEmail AsiaPacific@bbc.co.uk,`},
	} {
		t.Run(tc.text, func(t *testing.T) {
			if got := (PunctuationOptions{ReplacePeriods: true}).Apply(tc.text); got != tc.want {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
		})
	}
}

func TestTrailingCleanupKeepsLiteralSuffixes(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"AsiaPacific@bbc.co.uk", "AsiaPacific@bbc.co.uk"},
		{"https://example.test/path/", "https://example.test/path/"},
		{"Dr.", "Dr."},
		{"Use [2001:db8::]", "Use [2001:db8::]"},
		{"Use ::", "Use ::"},
		{"3.14.", "3.14"},
		{"README.md.", "README.md"},
		{"Edit .gitignore.", "Edit .gitignore"},
		{"Use data/sub/", "Use data/sub/"},
		{"他说“发到AsiaPacific@bbc.co.uk。”", "他说“发到AsiaPacific@bbc.co.uk”"},
		{"Wait...", "Wait"},
		{`<i>3.14.</i>\NAsiaPacific@bbc.co.uk.`, `<i>3.14</i>\NAsiaPacific@bbc.co.uk`},
	} {
		if got := (PunctuationOptions{StripTrailing: true, ReplacePeriods: true}).Apply(tc.text); got != tc.want {
			t.Errorf("%q => %q; want %q", tc.text, got, tc.want)
		}
	}
}
