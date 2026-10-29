// Constant buffer preserves source RGBA f32 words, with no byte texture or HSL
// conversion. Separate PS entry points expose the operation decomposition.
cbuffer Params : register(b0) { float4 wash; };
float4 vs(uint id : SV_VertexID) : SV_Position {
    float2 positions[3] = { float2(-1,-1), float2(-1,3), float2(3,-1) };
    return float4(positions[id], 0, 1);
}
float4 straight_ps(float4 position : SV_Position) : SV_Target0 {
    return wash;
}
float4 premul_ps(float4 position : SV_Position) : SV_Target0 {
    return float4(wash.rgb * wash.a, wash.a);
}
struct DualOutput { float4 color : SV_Target0; float4 alpha : SV_Target1; };
DualOutput dual_ps(float4 position : SV_Position) {
    DualOutput output;
    output.color = float4(wash.rgb * wash.a, wash.a);
    output.alpha = wash.aaaa;
    return output;
}
