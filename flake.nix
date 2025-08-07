{
  description = "RMI API Development Environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = true;
          };
        };

        pythonEnv = pkgs.python3.withPackages (ps: with ps; [
          pandas
          matplotlib
          seaborn
        ]);
      in {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            go-tools       # includes swag
            jq
            just
            k6
            docker
            docker-compose
            pythonEnv      # Python with plotting libs
            mongosh        # MongoDB Shell (standalone)
            redis
          ];

          shellHook = ''
            echo "RMI API Development Environment"
            echo "Available tools:"
            echo "- Go $(go version)"
            echo "- Just $(just --version)"
            echo "- K6 $(k6 --version)"
            echo "- MongoDB shell $(mongosh --version)"
            echo "- Redis server $(redis-server --version)"
            echo "- Docker $(docker --version)"
            echo "- Python $(python3 --version)"
            echo ""
            echo "To see available commands, run: just"

            # Add GOBIN to PATH
            export GOBIN="$PWD/bin"
            export PATH="$GOBIN:$PATH"
          '';
        };
      });
}
