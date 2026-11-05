{ stdenv, pkgs, lib }:

pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    (rust-bin.stable.latest.default.override { extensions = ["rust-src" "rust-analyzer-preview"]; })
    stdenv.cc.cc.lib
    pkg-config
    openssl
    yq-go

    # Golang
    # Keep this golang version in sync with the version in .tool-versions please
    go_1_25
    gopls
    delve
    golangci-lint
    gotools

    nodejs_24
  ];

  RUST_BACKTRACE = "1";

  LD_LIBRARY_PATH = lib.makeLibraryPath [pkgs.zlib stdenv.cc.cc.lib]; # lib64

  # Avoids issues with delve
  CGO_CPPFLAGS="-U_FORTIFY_SOURCE -D_FORTIFY_SOURCE=0";

  # Seems required to make the anchor image work
  DOCKER_DEFAULT_PLATFORM="linux/amd64";
}
