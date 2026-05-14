//go:build cloud

package systray

import "context"

func Run(shutdown func(ctx context.Context) error) {}
func Quit()                                        {}
