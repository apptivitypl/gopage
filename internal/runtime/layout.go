package runtime

const LayoutRoot = "layout"

func layered(props Accessible, planIndex int, opts *Options) Accessible {
	if planIndex >= len(opts.Layouts) || opts.Layouts[planIndex] == nil {
		return props
	}
	return WithRoot(props, LayoutRoot, opts.Layouts[planIndex])
}
