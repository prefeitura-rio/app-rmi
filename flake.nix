{
  description = "RMI API Development Environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = true;
            allowUnfreePredicate =
              pkg:
              builtins.elem (pkgs.lib.getName pkg) [
                "mongodb"
              ];
          };
        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            docker
            docker-compose
            go
            go-tools
            gopls
            gotools
            just
            k6
            mongodb
            redis
          ];

          shellHook = ''
            echo "RMI API Development Environment"
            echo "Available tools:"
            echo "- Go: $(go version)"
            echo "- Just: $(just --version)"
            echo "- MongoDB: $(mongod --version | head -n1)"
            echo "- Redis: $(redis-server --version)"
            echo "- Docker: $(docker --version)"
            echo "- k6: $(k6 version)"
            echo ""
            echo "To see available commands, run: just"

            export GOBIN="$PWD/bin"
            export PATH="$GOBIN:$PATH"
          '';
        };
      }
    );
}
