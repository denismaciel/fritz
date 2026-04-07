{
  description = "fritz coding agent harness";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";

  outputs =
    { self, nixpkgs }:
    let
      lib = nixpkgs.lib;
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          fritz = pkgs.callPackage ./package.nix { };
        in
        {
          inherit fritz;
          default = fritz;
        }
      );

      apps = forAllSystems (system: {
        fritz = {
          type = "app";
          program = "${self.packages.${system}.fritz}/bin/fritz";
        };
        default = self.apps.${system}.fritz;
      });

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gopls
              gotools
            ];
          };
        }
      );
    };
}
