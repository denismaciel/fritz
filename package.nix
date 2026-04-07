{
  buildGoModule,
  lib,
  pname ? "fritz",
  subPackage ? "cmd/fritz",
  mainProgram ? pname,
}:
buildGoModule {
  inherit pname;
  version = "0.1.0";

  src = ./.;

  subPackages = [ subPackage ];

  vendorHash = "sha256-EKRLp/yDYIvwQF1RvlTxhylnTzdnHoMvticQx8ONhlQ=";

  ldflags = [
    "-s"
    "-w"
  ];

  meta = with lib; {
    description = "Fritz CLI coding agent";
    inherit mainProgram;
    platforms = platforms.unix;
  };
}
