

[<img src="https://rclone.org/img/logo_on_light__horizontal_color.svg" width="50%" alt="rclone logo">](https://rclone.org/#gh-light-mode-only)
[<img src="https://rclone.org/img/logo_on_dark__horizontal_color.svg" width="50%" alt="rclone logo">](https://rclone.org/#gh-dark-mode-only)

[Website](https://rclone.org) |
[Documentation](https://rclone.org/docs/) |
[Download](https://rclone.org/downloads/) |
[Contributing](CONTRIBUTING.md) |
[Changelog](https://rclone.org/changelog/) |
[Installation](https://rclone.org/install/) |
[Forum](https://forum.rclone.org/)

# Rclone Out of Tree Example

This is an example of how to have an out of tree backend. You might want to do this before your backend is merged into rclone, or if it isn't suitable for merging (eg uses CGO) or for some other reason.

If you `git clone` this repository and build with `go build` it will make an executable which is rclone plus an additional backend called `ram`. This is a copy of the `memory` backend as an example.

This repo depends on rclone (check the `go.mod` for the version) and has one copied file from the rclone repo the `rclone.go` file where you import your backend.

The same technique will work for additional commands too.

License
-------

This is free software under the terms of the MIT license (check the [COPYING file](/COPYING) included in this package).
