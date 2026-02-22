{
  description = "Nile: durable, sandboxed, message-driven services";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils, ... }:
    let
      # NixOS module (system-independent)
      nixosModule = import ./modules/nile.nix;
    in
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "nile";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-AYissG+4cwerkFqpCY+efDrusoDXYE8RCnKTvxihUUc=";
          subPackages = [ "cmd/nile" ];

          meta = {
            description = "Nile runtime for durable message-driven services";
            mainProgram = "nile";
          };
        };

        packages.counter-service = pkgs.buildGoModule {
          pname = "counter-service";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-AYissG+4cwerkFqpCY+efDrusoDXYE8RCnKTvxihUUc=";
          subPackages = [ "examples/counter-service" ];
        };

        packages.nile-demo = pkgs.writeScriptBin "nile-demo" (builtins.readFile ./scripts/nile-demo);

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
          ];

          packages = [
            self.packages.${system}.default
            self.packages.${system}.counter-service
            self.packages.${system}.nile-demo
          ];

          NILE_DEMO_DIR = "/tmp/nile-demo";

          shellHook = ''
            mkdir -p $NILE_DEMO_DIR/{state,retain,dead,run,stream}
            echo "Nile dev shell — run 'nile-demo help' to get started"
          '';
        };
      }
    ) // {
      nixosModules.default = nixosModule;

      templates.service = {
        path = ./templates/service;
        description = "A new Nile service (copt)";
      };
    };
}
