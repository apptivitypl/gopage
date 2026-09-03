package postcard

type Props struct {
	Post Post
}

type Post struct {
	Slug    string
	Title   string
	Date    string
	Summary string
}
