export function supportPetFrameSize(width:number,height:number):{frame_width:number;frame_height:number}|null {
 if(!Number.isInteger(width)||!Number.isInteger(height)||width<8||height<9||width%8!==0||height%9!==0||width/8>1024||height/9>1024)return null
 return {frame_width:width/8,frame_height:height/9}
}
