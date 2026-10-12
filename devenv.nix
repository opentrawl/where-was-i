{ pkgs, ... }:

{
  cachix.enable = false;

  languages.go = {
    enable = true;
    package = pkgs.go_1_26;
    delve.enable = false;
    lsp.enable = false;
  };

  packages = [
    pkgs.buf
    pkgs.protoc-gen-go
  ];

  # Builds with Apple's own compiler and SDK, so the binary links only macOS libraries and runs on any Mac with
  # macOS 14 or later. The Nix toolchain would link its own libresolv from /nix/store.
  scripts.build.exec = ''
    env -u NIX_LDFLAGS -u NIX_CFLAGS_COMPILE -u SDKROOT -u DEVELOPER_DIR -u LD -u MACOSX_DEPLOYMENT_TARGET \
      CC=/usr/bin/clang CGO_ENABLED=1 \
      CGO_CFLAGS="-mmacosx-version-min=14.0" CGO_LDFLAGS="-mmacosx-version-min=14.0" \
      go build -trimpath -ldflags "-s -w" -o where-was-i ./cmd/where-was-i
  '';
}
