# go-ninja
[![forthebadge](http://forthebadge.com/badges/built-by-hipsters.svg)](http://forthebadge.com)
----------

A Golang library to interact with the Ninja Sphere-- used for creating tubular new drivers.

# Usage

Ensure you add a bugsnag to your drivers!

```go
func init() {
	bugsnag.Configure(bugsnag.Configuration{
		APIKey: config.BugsnagKey,
	})
}
```

See https://github.com/bugsnag/bugsnag-go for more information.

![go ninja go ninja go](http://cdn3.whatculture.com/wp-content/uploads/2013/05/vanilla-ice-ninja-turtles.jpg)


For development outside of a devkit/sphere, ensure you have sphere-config and sphere-serial in your path.
