package prompt

const PromptVersion = "v2"

type Builder struct {
	version string
}

func NewBuilder() *Builder {
	return &Builder{version: PromptVersion}
}

func (b *Builder) Build(in Inputs) Sections {
	if in.Article == nil {
		return nil
	}

	sections := make(Sections, 0, 6)
	sections = append(sections, b.buildCoreIdentity())
	sections = append(sections, b.buildTrustBoundary())
	sections = append(sections, b.buildScope())

	if hasValidFocus(in.Article) {
		sections = append(sections, b.buildFocus(in.Article))
	}
	if in.Article.ArticleContent != "" {
		sections = append(sections, b.buildArticlePayload(in.Article))
	}
	if len(in.Article.CodeBlocks) > 0 {
		sections = append(sections, b.buildCodeBlocksPayload(in.Article))
	}

	return sections
}

func hasValidFocus(article *ArticleContext) bool {
	return article.FocusedBlockIndex != nil &&
		*article.FocusedBlockIndex >= 0 &&
		*article.FocusedBlockIndex < len(article.CodeBlocks)
}
