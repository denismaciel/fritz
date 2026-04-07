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
          fritz-telegram = pkgs.callPackage ./package.nix {
            pname = "fritz-telegram";
            subPackage = "cmd/fritz-telegram";
            mainProgram = "fritz-telegram";
          };
        in
        {
          inherit fritz fritz-telegram;
          default = fritz;
        }
      );

      apps = forAllSystems (system: {
        fritz = {
          type = "app";
          program = "${self.packages.${system}.fritz}/bin/fritz";
        };
        fritz-telegram = {
          type = "app";
          program = "${self.packages.${system}.fritz-telegram}/bin/fritz-telegram";
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
