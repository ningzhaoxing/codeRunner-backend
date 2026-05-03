package prompt

type Inputs struct {
	SessionMode string
	Article     *ArticleContext
}

type ArticleContext struct {
	ArticleID         string
	ArticleContent    string
	CodeBlocks        []CodeBlock
	FocusedBlockIndex *int
}

type CodeBlock struct {
	Language string
	Code     string
}
