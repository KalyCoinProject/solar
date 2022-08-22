pragma solidity ^0.4.0;

contract KalyCoinTest {
   uint storedNumber;
   function KalyCoinTest() {
       storedNumber=1;
   }
   function setNumber(uint number) public {
       storedNumber = number;
   }
   function logNumber() constant public {
        log1("storedNumber", uintToBytes(storedNumber));
   }
   function returnNumber() constant public returns (uint) {
       return storedNumber;
   }
   function deposit() public payable {
   }
   function withdraw() public {
       if(!msg.sender.send(this.balance)) {
           throw;
       }
   }

   //utility function

   function uintToBytes(uint v) constant returns (bytes32 ret) {
       if (v == 0) {
           ret = "0";
       } else {
           while (v > 0) {
               ret = bytes32(uint(ret) / (2 ** 8));
               ret |= bytes32(((v % 10) + 48) * 2 ** (8 * 31));
               v /= 10;
           }
       }

       return ret;
   }
}