{ buildGoModule, lib }:
buildGoModule {
  pname = "fritz";
  version = "0.1.0";

  src = ./.;

  subPackages = [ "cmd/fritz" ];

  vendorHash = "sha256-EKRLp/yDYIvwQF1RvlTxhylnTzdnHoMvticQx8ONhlQ=";

  ldflags = [
    "-s"
    "-w"
  ];

  meta = with lib; {
    description = "Fritz CLI coding agent";
    mainProgram = "fritz";
    platforms = platforms.unix;
  };
}
