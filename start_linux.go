package launcher

import "github.com/alrusov/config"

//----------------------------------------------------------------------------------------------------------------------------//

/*
Nothing special to do.
A Linux service is simple, logical and self-sufficient.
*/

func start(a Application, cc *config.Common) {
	processor(a, cc)
}

//----------------------------------------------------------------------------------------------------------------------------//
