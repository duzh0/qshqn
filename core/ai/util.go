package ai

func partFromText(text string) Part {
	return Part{
		Text: text,
	}
}

func contentFromText(role Role, text string) Content {
	return Content{
		Role: role,
		Parts: []Part{
			partFromText(text),
		},
	}
}
