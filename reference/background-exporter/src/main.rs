mod effects;
fn source(w:u32,h:u32)->image::RgbaImage{image::RgbaImage::from_fn(w,h,|x,y|image::Rgba([((x*19+y*7)%256)as u8,((x*11+y*23)%256)as u8,((x*31+y*5)%256)as u8,((x*13+y*17)%256)as u8]))}
fn main()->Result<(),Box<dyn std::error::Error>>{
 let out=std::path::PathBuf::from(std::env::args().nth(1).expect("output directory"));std::fs::create_dir_all(&out)?;
 println!("luma coefficients: {:?}",moxcms::ColorPrimaries::BT_709.transform_to_xyz(moxcms::Chromaticity::D65).v[1]); let image=source(33,19); std::fs::write(out.join("source-luma.gray"),image::DynamicImage::ImageRgba8(image.clone()).to_luma8().as_raw())?;
 for effect in ["none","dither","ascii","halftone","scanlines"]{for light in [false,true]{let raster=effects::effect(&image,effect,light);std::fs::write(out.join(format!("{effect}-{}.rgba",if light{"light"}else{"dark"})),raster.as_raw())?;}}
 for (w,h,limit) in [(3,2,8),(19,11,7),(1,1,9),(9,3,5),(2,9,7)] {let proxy=image::DynamicImage::ImageRgba8(source(w,h)).thumbnail(limit,limit).to_rgba8();std::fs::write(out.join(format!("thumbnail-{w}-{h}-{limit}.rgba")),proxy.as_raw())?;println!("thumbnail {w}x{h}->{limit}: {}x{}",proxy.width(),proxy.height());}
 Ok(())
}
