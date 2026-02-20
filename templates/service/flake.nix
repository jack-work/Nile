{
  description = "My Nile service";

  inputs = {
    nile.url = "github:gluck/nile";
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nile, nixpkgs, ... }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
    in
    {
      # Build your neb binary however you like:
      packages.${system}.default = pkgs.writeShellScriptBin "my-service" ''
        # Minimal echo service — replace with your real build
        while IFS= read -r line; do
          method=$(echo "$line" | ${pkgs.jq}/bin/jq -r '.method // empty')
          id=$(echo "$line" | ${pkgs.jq}/bin/jq -r '.id')
          case "$method" in
            init|message|retain|shutdown)
              echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
              ;;
            *)
              echo "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32601,\"message\":\"unknown method\"},\"id\":$id}"
              ;;
          esac
        done
      '';

      nixosModules.default = {
        imports = [ nile.nixosModules.default ];

        services.nile.package = nile.packages.${system}.default;
        services.nile.copts.my-service = {
          package = self.packages.${system}.default;
          retention.maxMessages = 5000;
        };
      };
    };
}
